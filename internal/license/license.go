// Package license is the client for the EONYX license server
// (https://eonyx.tech/api/v1/{activate,verify,deactivate}). Responses are
// Ed25519-signed over canonical JSON; this client pins the server's public key
// and accepts nothing signed by any other key.
package license

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"eonyx/internal/config"
)

// pinnedSPKIBase64 is the production Ed25519 public key of the EONYX license
// server (SPKI DER, base64; fingerprint 2a29391274f4816e). Signed responses
// are accepted only from this key.
const pinnedSPKIBase64 = "MCowBQYDK2VwAyEAMo2jdOUGIynmscTMZFK0C7EO28zYitMJIlWmk6GoBBQ="

const defaultBaseURL = "https://eonyx.tech/api/v1"

// verifyKey is the parsed pinned key; swapped only by tests in this package.
var verifyKey = mustPinnedKey()

func mustPinnedKey() ed25519.PublicKey {
	der, err := base64.StdEncoding.DecodeString(pinnedSPKIBase64)
	if err != nil || len(der) < ed25519.PublicKeySize {
		panic("license: bad pinned public key")
	}
	return ed25519.PublicKey(der[len(der)-ed25519.PublicKeySize:])
}

// Fingerprint returns the first 16 hex chars of sha256 over the pinned SPKI
// DER — the same value the server's /admin dashboard shows.
func Fingerprint() string {
	der, _ := base64.StdEncoding.DecodeString(pinnedSPKIBase64)
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])[:16]
}

// State is the locally persisted activation, written after a successful
// activate and refreshed on every successful online verify.
type State struct {
	LicenseKey   string         `json:"license_key"`
	MachineID    string         `json:"machine_id"`
	LicenseFile  map[string]any `json:"license_file"`
	LicenseType  string         `json:"license_type"`
	ValidUntil   int64          `json:"valid_until"`
	LastVerified int64          `json:"last_verified"`
}

func baseURL() string {
	if u := os.Getenv("EONYX_LICENSE_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return defaultBaseURL
}

// StatePath returns where the activation is stored: next to the global data
// file, or EONYX_LICENSE_FILE when set.
func StatePath() string {
	if p := os.Getenv("EONYX_LICENSE_FILE"); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(config.GlobalDataPath()), "license.json")
}

// MachineID derives a stable identifier for this machine (hostname, user and
// OS, hashed), so re-activating on the same machine reuses its binding.
func MachineID() string {
	host, _ := os.Hostname()
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	sum := sha256.Sum256([]byte("eonyx-machine|" + host + "|" + name + "|" + runtime.GOOS))
	return hex.EncodeToString(sum[:])[:32]
}

// Load reads the persisted activation; a missing file returns (nil, nil).
func Load() (*State, error) {
	raw, err := os.ReadFile(StatePath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("license file is corrupt: %w", err)
	}
	return &st, nil
}

func (st *State) save() error {
	path := StatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// verifySigned checks payload's hex "signature" over the canonical bytes of
// its remaining fields against the pinned key.
func verifySigned(payload map[string]any) error {
	sigHex, _ := payload["signature"].(string)
	if sigHex == "" {
		return errors.New("response is not signed")
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return errors.New("malformed response signature")
	}
	b, err := signableBytes(payload)
	if err != nil {
		return err
	}
	if !ed25519.Verify(verifyKey, b, sig) {
		return errors.New("response signature does not match the pinned EONYX key")
	}
	return nil
}

func newNonce() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// post sends one API call and decodes the JSON object. Non-200 responses
// (rate limit, replay guard) carry a {"detail": ...} body.
func post(ctx context.Context, path string, body map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL()+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("license server returned invalid JSON (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		detail, _ := out["detail"].(string)
		if detail == "" {
			detail = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("license server: %s", detail)
	}
	return out, nil
}

func requestBody(key, machineID string, withHW bool) map[string]any {
	body := map[string]any{
		"license_key": key,
		"machine_id":  machineID,
		"adapter_id":  "",
		"nonce":       newNonce(),
		"timestamp":   time.Now().Unix(),
	}
	if withHW {
		body["hw_components"] = map[string]any{
			"app":  "eonyx-cli",
			"os":   runtime.GOOS,
			"arch": runtime.GOARCH,
		}
	}
	return body
}

// Activate binds the license key to this machine and persists the signed
// license file locally.
func Activate(ctx context.Context, key string) (*State, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	machineID := MachineID()
	resp, err := post(ctx, "/activate", requestBody(key, machineID, true))
	if err != nil {
		return nil, err
	}
	if status, _ := resp["status"].(string); status != "ok" {
		msg, _ := resp["error"].(string)
		if msg == "" {
			msg = "activation refused"
		}
		return nil, errors.New(msg)
	}
	if err := verifySigned(resp); err != nil {
		return nil, err
	}
	lf, _ := resp["license_file"].(map[string]any)
	if lf == nil {
		return nil, errors.New("activation response has no license file")
	}
	if err := verifySigned(lf); err != nil {
		return nil, fmt.Errorf("license file: %w", err)
	}
	licType, _ := lf["license_type"].(string)
	validUntil, _ := lf["valid_until"].(float64)
	st := &State{
		LicenseKey:   key,
		MachineID:    machineID,
		LicenseFile:  lf,
		LicenseType:  licType,
		ValidUntil:   int64(validUntil),
		LastVerified: time.Now().Unix(),
	}
	if err := st.save(); err != nil {
		return nil, err
	}
	return st, nil
}

// VerifyOnline asks the server whether the stored activation is still good.
// ok=false with rejected=true means the server rejected it definitively (the
// rejection is signature-checked); err covers network and untrusted responses.
func VerifyOnline(ctx context.Context, st *State) (ok bool, rejected bool, serverMsg string, err error) {
	resp, err := post(ctx, "/verify", requestBody(st.LicenseKey, st.MachineID, false))
	if err != nil {
		return false, false, "", err
	}
	status, _ := resp["status"].(string)
	if status == "ok" {
		if err := verifySigned(resp); err != nil {
			return false, false, "", err
		}
		licType, _ := resp["license_type"].(string)
		validUntil, _ := resp["valid_until"].(float64)
		st.LicenseType = licType
		st.ValidUntil = int64(validUntil)
		st.LastVerified = time.Now().Unix()
		if err := st.save(); err != nil {
			return false, false, "", err
		}
		return true, false, "", nil
	}
	// Error responses sign {status, error} only.
	msg, _ := resp["error"].(string)
	signed := map[string]any{"status": status, "error": msg}
	if sig, ok := resp["signature"].(string); ok {
		signed["signature"] = sig
	}
	if err := verifySigned(signed); err != nil {
		return false, false, "", err
	}
	if msg == "" {
		msg = "license rejected"
	}
	return false, true, msg, nil
}

// Deactivate frees this machine's binding on the server and removes the local
// activation file.
func Deactivate(ctx context.Context) error {
	st, err := Load()
	if err != nil {
		return err
	}
	if st == nil {
		return errors.New("no activation on this machine")
	}
	resp, err := post(ctx, "/deactivate", requestBody(st.LicenseKey, st.MachineID, false))
	if err != nil {
		return err
	}
	if status, _ := resp["status"].(string); status != "ok" {
		msg, _ := resp["error"].(string)
		if msg == "" {
			msg = "deactivation refused"
		}
		return errors.New(msg)
	}
	return os.Remove(StatePath())
}

// notActivatedErr is the actionable message shown when the agent is started
// without a valid activation.
func notActivatedErr(reason string) error {
	return fmt.Errorf(`%s

EONYX needs an activated license to run.
  Buy a license at https://eonyx.tech, then activate this machine:
    eonyx license activate XXXX-XXXX-XXXX-XXXX`, reason)
}

// Ensure gates agent startup: it requires a locally stored, signature-valid
// activation and a successful online verification — starting without internet
// access is not supported.
func Ensure(ctx context.Context) error {
	st, err := Load()
	if err != nil {
		return notActivatedErr(err.Error())
	}
	if st == nil {
		return notActivatedErr("no license is activated on this machine.")
	}
	if st.MachineID != MachineID() {
		return notActivatedErr("the stored license file belongs to a different machine.")
	}
	if err := verifySigned(st.LicenseFile); err != nil {
		return notActivatedErr("stored license file failed verification: " + err.Error())
	}
	now := time.Now()
	if st.ValidUntil > 0 && now.Unix() > st.ValidUntil {
		return notActivatedErr(fmt.Sprintf("the license expired on %s.",
			time.Unix(st.ValidUntil, 0).Format("2006-01-02")))
	}

	vctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	ok, _, serverMsg, verr := VerifyOnline(vctx, st)
	if verr != nil {
		return fmt.Errorf("cannot reach the EONYX license server: %v\nAn internet connection is required to start.", verr)
	}
	if !ok {
		return notActivatedErr("the license server rejected this activation: " + serverMsg + ".")
	}
	return nil
}
