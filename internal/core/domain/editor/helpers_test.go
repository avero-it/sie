package editor

import (
	"strings"
	"testing"

	"github.com/avero-it/sie/internal/core/domain/buffer"
)

// doc builds a document from s, where '|' marks the cursor and an optional
// '^' marks the selection anchor. Neither marker may otherwise appear in s.
func doc(t *testing.T, s string) *Document {
	t.Helper()
	head := strings.IndexByte(s, '|')
	if head < 0 {
		t.Fatalf("doc %q: missing cursor marker", s)
	}
	anchor := strings.IndexByte(s, '^')
	text := strings.NewReplacer("|", "", "^", "").Replace(s)
	switch {
	case anchor < 0:
		anchor = head
	case anchor < head:
		head--
	default:
		anchor--
	}
	d := NewDocument([]byte(text), Options{TabWidth: 4, DefaultEOL: buffer.LF})
	d.SetSelection(Selection{anchor, head})
	return d
}

// show renders d's text with the markers used by doc.
func show(d *Document) string {
	text := d.Text()
	sel := d.Selection()
	type mark struct {
		off int
		s   string
	}
	marks := []mark{{sel.Head, "|"}}
	if !sel.Empty() {
		marks = append(marks, mark{sel.Anchor, "^"})
		if sel.Anchor < sel.Head {
			marks[0], marks[1] = marks[1], marks[0]
		}
	}
	var b strings.Builder
	prev := 0
	for _, m := range marks {
		b.WriteString(text[prev:m.off])
		b.WriteString(m.s)
		prev = m.off
	}
	b.WriteString(text[prev:])
	return b.String()
}

func TestDocHelpersRoundTrip(t *testing.T) {
	for _, s := range []string{"|", "ab|c", "a^bc|d", "a|bc^d", "x\n|y"} {
		if got := show(doc(t, s)); got != s {
			t.Errorf("show(doc(%q)) = %q", s, got)
		}
	}
}
