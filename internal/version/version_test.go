package version

import (
	"strings"
	"testing"
)

func TestStringReturnsInjectedVersion(t *testing.T) {
	old := Version
	defer func() { Version = old }()

	Version = "v1.2.3"
	if got := String(); got != "v1.2.3" {
		t.Errorf("String() = %q, want v1.2.3", got)
	}
}

func TestStringFallsBackWhenDev(t *testing.T) {
	old := Version
	defer func() { Version = old }()

	Version = "dev"
	got := String()
	if got == "" {
		t.Fatal("String() should never be empty")
	}
	// Without an injected tag the value is either a real embedded tag ("vX.Y.Z")
	// or the derived "dev"/"dev-<rev>" form — never the raw pseudo-version.
	if strings.HasPrefix(got, "v0.0.0-") {
		t.Errorf("String() = %q, should not expose the pseudo-version", got)
	}
	if !strings.HasPrefix(got, "dev") && !strings.HasPrefix(got, "v") {
		t.Errorf("String() = %q, want a dev* or v* value", got)
	}
}
