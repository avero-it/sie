package clipboard

import (
	"errors"
	"testing"
)

type fakeNative struct {
	text string
	err  error
}

func (f *fakeNative) copy(s string) error    { f.text = s; return f.err }
func (f *fakeNative) paste() (string, error) { return f.text, f.err }

func TestCopyUsesOSC52AndNative(t *testing.T) {
	var sent []byte
	n := &fakeNative{}
	c := &Clipboard{osc52: func(b []byte) { sent = b }, native: n}
	if err := c.Copy("héllo"); err != nil {
		t.Fatal(err)
	}
	if string(sent) != "héllo" || n.text != "héllo" {
		t.Errorf("osc52 %q native %q", sent, n.text)
	}
	if got, err := c.Paste(); err != nil || got != "héllo" {
		t.Errorf("paste %q %v", got, err)
	}
}

func TestNativeFailureIsMaskedByOSC52(t *testing.T) {
	c := &Clipboard{osc52: func([]byte) {}, native: &fakeNative{err: errors.New("boom")}}
	if err := c.Copy("x"); err != nil {
		t.Errorf("copy with working OSC52 failed: %v", err)
	}
}

func TestNothingAvailable(t *testing.T) {
	c := &Clipboard{}
	if err := c.Copy("x"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("copy err %v", err)
	}
	if _, err := c.Paste(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("paste err %v", err)
	}
}

func TestOSC52OnlyCopySucceedsPasteFails(t *testing.T) {
	c := &Clipboard{osc52: func([]byte) {}}
	if err := c.Copy("x"); err != nil {
		t.Error(err)
	}
	if _, err := c.Paste(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("paste err %v", err)
	}
}
