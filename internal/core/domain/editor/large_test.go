package editor

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// fileSource is a buffer.Source that is not in memory.
type fileSource struct{ data []byte }

func (f fileSource) Len() int { return len(f.data) }

func (f fileSource) ReadAt(dst []byte, off int64) (int, error) {
	n := copy(dst, f.data[off:])
	if n < len(dst) {
		return n, io.EOF
	}
	return n, nil
}

func TestSearchAcrossWindows(t *testing.T) {
	filler := strings.Repeat("abcdefghi\n", 3*searchWindow/10)
	// Matches straddling a window boundary, in the middle and near the end.
	for _, at := range []int{searchWindow - 3, 2*searchWindow - 1, len(filler) - 10} {
		text := filler[:at] + "NEEDLE" + filler[at+6:]
		for _, mk := range []string{"memory", "file"} {
			t.Run(mk, func(t *testing.T) {
				d := docFor(t, mk, text)
				for _, opts := range []SearchOptions{{CaseSensitive: true}, {}} {
					q := "NEEDLE"
					if !opts.CaseSensitive {
						q = "needle"
					}
					d.SetSelection(Cursor(0))
					if !d.Find(q, true, opts) || d.Selection().Anchor != at {
						t.Fatalf("forward from 0 (%v): sel %+v, want start %d", opts, d.Selection(), at)
					}
					d.SetSelection(Cursor(len(text)))
					if !d.Find(q, false, opts) || d.Selection().Anchor != at {
						t.Fatalf("backward from end (%v): sel %+v, want start %d", opts, d.Selection(), at)
					}
				}
			})
		}
	}
}

func docFor(t *testing.T, kind, text string) *Document {
	t.Helper()
	if kind == "memory" {
		return NewDocument([]byte(text), Options{})
	}
	d, err := NewDocumentFromSource(fileSource{[]byte(text)}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestReplaceAllAcrossWindows(t *testing.T) {
	text := []byte(strings.Repeat("abcdefghi\n", 2*searchWindow/10))
	for _, at := range []int{5, searchWindow - 3, searchWindow + 10, 2*searchWindow - 10} {
		copy(text[at:], "NEEDLE")
	}
	d := docFor(t, "file", string(text))
	n, err := d.ReplaceAll("NEEDLE", "x", SearchOptions{CaseSensitive: true})
	want := strings.ReplaceAll(string(text), "NEEDLE", "x")
	if err != nil || n != 4 || d.Text() != want {
		t.Fatalf("ReplaceAll = %d, %v; text matches: %v", n, err, d.Text() == want)
	}
}

func TestDocumentFromSource(t *testing.T) {
	body := "one\r\ntwo\r\nthree"
	for _, bom := range []bool{false, true} {
		raw := []byte(body)
		if bom {
			raw = append([]byte("\xef\xbb\xbf"), raw...)
		}
		d, err := NewDocumentFromSource(fileSource{raw}, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if d.HasBOM() != bom || d.LineEnding().String() != "CRLF" || d.LineCount() != 3 || d.Text() != body {
			t.Fatalf("bom=%v: HasBOM %v eol %v lines %d text %q", bom, d.HasBOM(), d.LineEnding(), d.LineCount(), d.Text())
		}
		if err := d.Insert("X"); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if _, err := d.WriteTo(&out); err != nil || !bytes.Equal(out.Bytes(), d.Bytes()) {
			t.Fatalf("WriteTo = %q, %v; Bytes %q", out.Bytes(), err, d.Bytes())
		}
		saved := bytes.Clone(out.Bytes())
		if err := d.Rebase(fileSource{saved}); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(d.Bytes(), saved) || d.Err() != nil {
			t.Fatalf("after Rebase: %q, err %v", d.Bytes(), d.Err())
		}
		if _, err := d.Undo(); err != nil || d.Text() != body {
			t.Fatalf("undo after Rebase: %q, %v", d.Text(), err)
		}
	}
}

func TestHugeChangeIsAppliedWithoutUndo(t *testing.T) {
	text := strings.Repeat("0123456789\n", 100) // 1100 bytes
	d := NewDocument([]byte(text), Options{UndoBudget: 500})
	d.SetSelection(Cursor(0))
	_ = d.Insert("small ")
	d.SetSelection(Selection{0, d.Len()})
	if err := d.DeleteSelection(); err != nil { // 1106 bytes > budget
		t.Fatal(err)
	}
	if d.Len() != 0 || d.CanUndo() || !d.Modified() || !d.TakeUndoLost() || d.TakeUndoLost() {
		t.Errorf("len=%d canUndo=%v modified=%v", d.Len(), d.CanUndo(), d.Modified())
	}
	// Normal editing carries on, with undo.
	_ = d.Insert("ok")
	if ok, err := d.Undo(); !ok || err != nil || d.Text() != "" {
		t.Errorf("undo after: %v %v %q", ok, err, d.Text())
	}
}
