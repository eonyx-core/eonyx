package report

import "testing"

func TestCSVEscape(t *testing.T) {
	cases := map[string]string{
		"plain":        "plain",
		"with,comma":   `"with,comma"`,
		"with\"quote":  `"with""quote"`,
		"with\nnewine": "\"with\nnewine\"",
	}
	for in, want := range cases {
		if got := csvEscape(in); got != want {
			t.Errorf("csvEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNullableString(t *testing.T) {
	if got := nullableString(nil); got != "NULL" {
		t.Errorf("nullableString(nil) = %q, want NULL", got)
	}
	s := "dept"
	if got := nullableString(&s); got != "dept" {
		t.Errorf("nullableString(&s) = %q, want dept", got)
	}
}

func TestNullableFloat(t *testing.T) {
	if got := nullableFloat(nil, 2); got != "NULL" {
		t.Errorf("nullableFloat(nil) = %q, want NULL", got)
	}
	v := 3.14159
	if got := nullableFloat(&v, 2); got != "3.14" {
		t.Errorf("nullableFloat(3.14159, 2) = %q, want 3.14", got)
	}
	if got := nullableFloat(&v, 1); got != "3.1" {
		t.Errorf("nullableFloat(3.14159, 1) = %q, want 3.1", got)
	}
}

func TestCSVBuilder(t *testing.T) {
	var b csvBuilder
	b.row([]string{"a", "b"})
	b.row([]string{"1", "two,2"})
	want := "a,b\n1,\"two,2\""
	if got := b.String(); got != want {
		t.Errorf("csvBuilder = %q, want %q", got, want)
	}
}
