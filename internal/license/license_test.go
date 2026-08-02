package license

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPinnedKeyFingerprint(t *testing.T) {
	if got := Fingerprint(); got != "2a29391274f4816e" {
		t.Fatalf("pinned key fingerprint = %s, want 2a29391274f4816e", got)
	}
}

func TestCanonicalJSON(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{map[string]any{"b": int64(1), "a": "x"}, `{"a":"x","b":1}`},
		{map[string]any{"a": "т\n"}, `{"a":"\u0442\n"}`},
		{map[string]any{"a": "😀"}, `{"a":"\ud83d\ude00"}`},
		{map[string]any{"s": `a"\`}, `{"s":"a\"\\"}`},
		{map[string]any{"n": nil, "t": true, "f": false}, `{"f":false,"n":null,"t":true}`},
		{map[string]any{"x": float64(1755000000)}, `{"x":1755000000}`},
		{map[string]any{"o": map[string]any{"z": int64(1), "a": []any{"q", int64(2)}}}, `{"o":{"a":["q",2],"z":1}}`},
	}
	for _, c := range cases {
		var b strings.Builder
		if err := canonicalJSON(c.in, &b); err != nil {
			t.Fatalf("canonicalJSON(%v): %v", c.in, err)
		}
		if b.String() != c.want {
			t.Errorf("canonicalJSON(%v) = %s, want %s", c.in, b.String(), c.want)
		}
	}
}

func TestCanonicalJSONRejectsFloat(t *testing.T) {
	var b strings.Builder
	if err := canonicalJSON(map[string]any{"x": 1.5}, &b); err == nil {
		t.Fatal("expected error for non-integer float")
	}
}

func TestSignableBytesExcludesSignature(t *testing.T) {
	b, err := signableBytes(map[string]any{"a": "x", "signature": "deadbeef"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"a":"x"}` {
		t.Fatalf("signableBytes = %s", b)
	}
}

// signPayload signs like the server: Ed25519 over canonical bytes of all
// fields except "signature", hex-encoded.
func signPayload(t *testing.T, priv ed25519.PrivateKey, m map[string]any) string {
	t.Helper()
	b, err := signableBytes(m)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(ed25519.Sign(priv, b))
}

func withTestKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	old := verifyKey
	verifyKey = pub
	t.Cleanup(func() { verifyKey = old })
	return priv
}

func TestVerifySignedRoundtrip(t *testing.T) {
	priv := withTestKey(t)
	m := map[string]any{"status": "ok", "error": "", "valid_until": float64(0)}
	m["signature"] = signPayload(t, priv, m)
	if err := verifySigned(m); err != nil {
		t.Fatalf("verifySigned: %v", err)
	}
	m["status"] = "tampered"
	if err := verifySigned(m); err == nil {
		t.Fatal("tampered payload must not verify")
	}
}

func TestMachineIDStable(t *testing.T) {
	a, b := MachineID(), MachineID()
	if a != b || len(a) != 32 {
		t.Fatalf("MachineID unstable or wrong length: %q vs %q", a, b)
	}
}

// mock license server implementing activate/verify with a swappable verdict.
func mockServer(t *testing.T, priv ed25519.PrivateKey, verdict *string) *httptest.Server {
	t.Helper()
	future := time.Now().Add(30 * 24 * time.Hour).Unix()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		var resp map[string]any
		switch {
		case strings.HasSuffix(r.URL.Path, "/activate"):
			lf := map[string]any{
				"license_key":   body["license_key"],
				"machine_id":    body["machine_id"],
				"adapter_id":    body["adapter_id"],
				"hw_components": body["hw_components"],
				"license_type":  "subscription",
				"product":       "all",
				"valid_until":   future,
				"issued_at":     time.Now().Unix(),
			}
			lf["signature"] = signPayload(t, priv, lf)
			resp = map[string]any{"status": "ok", "error": "", "license_file": lf}
			resp["signature"] = signPayload(t, priv, resp)
		case strings.HasSuffix(r.URL.Path, "/verify"):
			if *verdict == "ok" {
				resp = map[string]any{"status": "ok", "error": "", "license_type": "subscription", "valid_until": future}
				resp["signature"] = signPayload(t, priv, resp)
			} else {
				signed := map[string]any{"status": "error", "error": *verdict}
				sig := signPayload(t, priv, signed)
				resp = map[string]any{"status": "error", "error": *verdict, "license_type": "", "valid_until": float64(0), "signature": sig}
			}
		case strings.HasSuffix(r.URL.Path, "/deactivate"):
			resp = map[string]any{"status": "ok", "error": ""}
			resp["signature"] = signPayload(t, priv, resp)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestActivateVerifyEnsureFlow(t *testing.T) {
	priv := withTestKey(t)
	verdict := "ok"
	srv := mockServer(t, priv, &verdict)
	defer srv.Close()

	t.Setenv("EONYX_LICENSE_URL", srv.URL)
	t.Setenv("EONYX_LICENSE_FILE", filepath.Join(t.TempDir(), "license.json"))

	ctx := context.Background()
	st, err := Activate(ctx, "abcd-1234-abcd-1234")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if st.LicenseKey != "ABCD-1234-ABCD-1234" {
		t.Errorf("key not normalized: %s", st.LicenseKey)
	}
	if st.LicenseType != "subscription" || st.ValidUntil == 0 {
		t.Errorf("state not filled: %+v", st)
	}

	if err := Ensure(ctx); err != nil {
		t.Fatalf("Ensure after activate: %v", err)
	}

	verdict = "License has been revoked"
	if err := Ensure(ctx); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("Ensure with revoked license = %v, want revoked error", err)
	}

	// Server gone: startup requires a live online verification, so Ensure fails.
	verdict = "ok"
	if err := Ensure(ctx); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	srv.Close()
	if err := Ensure(ctx); err == nil || !strings.Contains(err.Error(), "internet connection") {
		t.Fatalf("Ensure with unreachable server = %v, want connectivity error", err)
	}
}

func TestEnsureWithoutActivation(t *testing.T) {
	t.Setenv("EONYX_LICENSE_FILE", filepath.Join(t.TempDir(), "license.json"))
	err := Ensure(context.Background())
	if err == nil || !strings.Contains(err.Error(), "eonyx license activate") {
		t.Fatalf("Ensure without state = %v, want activation instructions", err)
	}
}
