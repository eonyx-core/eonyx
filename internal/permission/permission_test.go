package permission

import "testing"

func TestAuthorize(t *testing.T) {
	p := New([]string{"bash"}, false)

	for _, name := range []string{"read", "ls", "glob", "grep"} {
		if err := p.Authorize(name); err != nil {
			t.Errorf("safe tool %q refused: %v", name, err)
		}
	}
	if err := p.Authorize("bash"); err != nil {
		t.Errorf("allowlisted bash refused: %v", err)
	}
	if err := p.Authorize("write"); err == nil {
		t.Error("write should be refused without allowlist or yolo")
	}
}

func TestYoloAllowsEverything(t *testing.T) {
	p := New(nil, true)
	for _, name := range []string{"write", "edit", "bash", "fetch"} {
		if err := p.Authorize(name); err != nil {
			t.Errorf("yolo refused %q: %v", name, err)
		}
	}
}
