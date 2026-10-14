package app

import (
	"testing"

	"github.com/avero-it/sie/internal/core/ports"
)

func newSaveHarness(t *testing.T, files map[string]string) *harness {
	t.Helper()
	m := &memFS{files: map[string][]byte{}}
	for k, v := range files {
		m.files[k] = []byte(v)
	}
	e, err := New(Config{Files: m, Keymap: keys, Version: "test", Validator: fakeValidator{},
		Highlighter: &fakeHighlighter{}, ValidateOnSave: true})
	if err != nil {
		t.Fatal(err)
	}
	e.Resize(100, 10)
	return &harness{Editor: e, t: t, fs: m}
}

const choiceRow = "Fails validation (line 2, col 4: bad thing)  [v]alidate  [s]ave anyway  Esc"

func TestSaveValidFileSavesDirectly(t *testing.T) {
	h := newSaveHarness(t, map[string]string{"a.yaml": "a: 1\n"})
	_ = h.Dispatch("file.open", "a.yaml")
	h.typeText("x")
	h.keys(ports.Ctrl('s'))
	if string(h.fs.files["a.yaml"]) != "xa: 1\n" || h.focus != focusEditor {
		t.Fatalf("not saved: %q focus %v", h.fs.files["a.yaml"], h.focus)
	}
}

func TestSaveInvalidAsks(t *testing.T) {
	setup := func() *harness {
		h := newSaveHarness(t, map[string]string{"a.yaml": "a: 1\nb: BAD\n"})
		_ = h.Dispatch("file.open", "a.yaml")
		h.typeText("#")
		h.keys(ports.Ctrl('s'))
		h.wantRow(rowPrompt, choiceRow)
		return h
	}
	// s: save anyway.
	h := setup()
	h.typeText("s")
	if string(h.fs.files["a.yaml"]) != "#a: 1\nb: BAD\n" {
		t.Errorf("save anyway did not write: %q", h.fs.files["a.yaml"])
	}
	h.wantRowPrefix(rowPrompt, "> Save file — wrote 3 lines")

	// v: don't save, jump to the problem.
	h = setup()
	h.typeText("v")
	if string(h.fs.files["a.yaml"]) != "a: 1\nb: BAD\n" {
		t.Errorf("validate wrote the file: %q", h.fs.files["a.yaml"])
	}
	h.wantRow(rowPrompt, "Validate: line 2, col 4: bad thing")
	if line, col, _ := h.doc.CursorPos(); line != 1 || col != 3 {
		t.Errorf("cursor %d:%d", line, col)
	}
	if !h.doc.Modified() {
		t.Error("document marked saved")
	}

	// Esc: nothing.
	h = setup()
	h.keys(key(ports.KeyEsc))
	h.wantRow(rowPrompt, "Cancelled")
	if string(h.fs.files["a.yaml"]) != "a: 1\nb: BAD\n" {
		t.Error("Esc wrote the file")
	}

	// Every save asks again, even right after "save anyway".
	h = setup()
	h.typeText("s")
	h.typeText("!")
	h.keys(ports.Ctrl('s'))
	h.wantRow(rowPrompt, choiceRow)
}

func TestSaveAsValidatesByTargetType(t *testing.T) {
	h := newSaveHarness(t, map[string]string{"data.txt": "{\n BAD\n"})
	_ = h.Dispatch("file.open", "data.txt")
	h.keys(ports.Ctrl('s')) // plain text: saved without validation
	if h.focus != focusEditor {
		t.Fatal("plain text was validated")
	}
	_ = h.Dispatch("file.saveAs", "data.json") // becomes JSON: validated first
	h.wantRowPrefix(rowPrompt, "Fails validation (line 2, col 2: bad thing)")
	h.typeText("s")
	if _, ok := h.fs.files["data.json"]; !ok {
		t.Error("save anyway did not write the new file")
	}
}

func TestValidateOnSaveCanBeTurnedOff(t *testing.T) {
	h := newSaveHarness(t, map[string]string{"a.yaml": "BAD\n"})
	_ = h.Dispatch("file.open", "a.yaml")
	_ = h.Dispatch("view.toggleValidateOnSave")
	h.wantRowPrefix(rowPrompt, ">   Validate on save — off")
	h.typeText("x")
	h.keys(ports.Ctrl('s'))
	if string(h.fs.files["a.yaml"]) != "xBAD\n" {
		t.Errorf("not saved with the option off: %q", h.fs.files["a.yaml"])
	}
}

func TestSaveAfterOverwriteConfirmStillValidates(t *testing.T) {
	h := newSaveHarness(t, map[string]string{"new.yaml": "old", "a.yaml": "a: BAD\n"})
	_ = h.Dispatch("file.open", "a.yaml")
	_ = h.Dispatch("file.saveAs", "new.yaml")
	h.wantRow(rowPrompt, "Overwrite new.yaml? (y/n)")
	h.typeText("y")
	h.wantRowPrefix(rowPrompt, "Fails validation (line 1, col 4: bad thing)")
	h.typeText("v")
	if string(h.fs.files["new.yaml"]) != "old" {
		t.Error("overwritten despite choosing validate")
	}
}
