package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"-version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "sie ") {
		t.Errorf("output %q", out.String())
	}
}

func TestTooManyArgs(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"a", "b"}, &out, &errOut); code != 2 {
		t.Errorf("exit %d", code)
	}
	if !strings.Contains(errOut.String(), "usage: sie") {
		t.Errorf("stderr %q", errOut.String())
	}
}
