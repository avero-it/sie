package editor

import (
	"bytes"
	"testing"
)

func FuzzDocument(f *testing.F) {
	f.Add([]byte("hello\r\nwörld\t日本\n"), []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})
	f.Add([]byte("é\xff\r"), []byte{3, 3, 9, 1, 12, 7, 14, 2})
	f.Fuzz(func(t *testing.T, initial, ops []byte) {
		d := NewDocument(bytes.Clone(initial), Options{TabWidth: 4})
		original := d.Bytes()
		inserts := []string{"x", "\n", "日", "é", "\t", "ab\r\ncd"}
		for i, op := range ops {
			var err error
			switch op % 16 {
			case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9:
				d.Move(Motion(op%10), op&0x10 != 0)
			case 10:
				err = d.Insert(inserts[i%len(inserts)])
			case 11:
				err = d.InsertNewline()
			case 12:
				err = d.Backspace()
			case 13:
				err = d.DeleteForward()
			case 14:
				switch (op >> 4) % 3 {
				case 0:
					err = d.DeleteLine()
				case 1:
					err = d.DuplicateLine()
				default:
					err = d.DeleteToLineEnd()
				}
			case 15:
				if op&0x10 != 0 {
					d.AddCursor(int(op) * 7 % (d.Len() + 1))
				} else {
					d.SetSelection(Selection{int(op) * 7 % (d.Len() + 1), i % (d.Len() + 1)})
				}
			}
			if err != nil {
				t.Fatalf("op %d (%d): %v", i, op, err)
			}
			sels := d.Selections()
			for j, sel := range sels {
				if d.snap(sel.Anchor) != sel.Anchor || d.snap(sel.Head) != sel.Head {
					t.Fatalf("op %d (%d): selection %+v not on a valid cursor position in %q", i, op, sel, d.Text())
				}
				for _, other := range sels[j+1:] {
					if overlaps(sel, other) {
						t.Fatalf("op %d (%d): selections %+v and %+v overlap", i, op, sel, other)
					}
				}
			}
		}
		for d.CanUndo() {
			if _, err := d.Undo(); err != nil {
				t.Fatal(err)
			}
		}
		if !bytes.Equal(d.Bytes(), original) {
			t.Fatalf("undo all: %q, want %q", d.Bytes(), original)
		}
	})
}
