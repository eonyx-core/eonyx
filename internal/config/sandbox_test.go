package config

import "testing"

func TestFileSandboxRoot(t *testing.T) {
	c := &Config{}
	if got := c.FileSandboxRoot("/wd"); got != "" {
		t.Errorf("disabled sandbox = %q, want empty", got)
	}

	c.Permissions.Sandbox = true
	if got := c.FileSandboxRoot("/wd"); got != "/wd" {
		t.Errorf("without a project root, sandbox root = %q, want /wd", got)
	}

	c.projectRoot = "/proj"
	if got := c.FileSandboxRoot("/wd"); got != "/proj" {
		t.Errorf("with a project root, sandbox root = %q, want /proj", got)
	}
}
