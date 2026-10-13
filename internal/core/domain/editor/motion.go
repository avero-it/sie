package editor

// Motion is a cursor movement.
type Motion uint8

const (
	MoveLeft Motion = iota
	MoveRight
	MoveUp
	MoveDown
	MoveWordLeft
	MoveWordRight
	MoveLineStart // smart home: first non-blank, then column 0
	MoveLineEnd
	MoveDocStart
	MoveDocEnd
)

// Move moves the cursor; with extend the selection anchor stays put.
// Without extend, Left/Right on a selection collapse it to its start/end.
func (d *Document) Move(m Motion, extend bool) {
	d.hist.Seal()
	if !extend && !d.sel.Empty() {
		start, end := d.sel.Range()
		switch m {
		case MoveLeft:
			d.collapse(start)
			return
		case MoveRight:
			d.collapse(end)
			return
		}
	}
	head := d.sel.Head
	vertical := false
	switch m {
	case MoveLeft:
		head = d.prevPos(head)
	case MoveRight:
		head = d.nextPos(head)
	case MoveUp:
		head, vertical = d.verticalPos(head, -1), true
	case MoveDown:
		head, vertical = d.verticalPos(head, 1), true
	case MoveWordLeft:
		head = d.wordLeft(head)
	case MoveWordRight:
		head = d.wordRight(head)
	case MoveLineStart:
		head = d.smartHome(head)
	case MoveLineEnd:
		_, head = d.lineRange(d.lineOf(head))
	case MoveDocStart:
		head = 0
	case MoveDocEnd:
		head = d.buf.Len()
	}
	if !vertical {
		d.goal = -1
	}
	d.setHead(head, extend)
}

// MovePage moves the cursor by rows lines (negative is up), keeping the
// preferred screen column.
func (d *Document) MovePage(rows int, extend bool) {
	d.hist.Seal()
	d.setHead(d.verticalPos(d.sel.Head, rows), extend)
}

func (d *Document) setHead(head int, extend bool) {
	if extend {
		d.sel.Head = head
		return
	}
	d.sel = Cursor(head)
}

func (d *Document) collapse(off int) {
	d.goal = -1
	d.sel = Cursor(off)
}

// prevPos returns the cursor position one grapheme (or line break) before off.
func (d *Document) prevPos(off int) int {
	line := d.lineOf(off)
	start, _ := d.lineRange(line)
	if off == start {
		if line == 0 {
			return 0
		}
		_, end := d.lineRange(line - 1)
		return end
	}
	return start + prevBoundary(d.Line(line), off-start)
}

// nextPos returns the cursor position one grapheme (or line break) after off.
func (d *Document) nextPos(off int) int {
	line := d.lineOf(off)
	start, end := d.lineRange(line)
	if off >= end {
		if line == d.LineCount()-1 {
			return off
		}
		return d.lineStart(line + 1)
	}
	return start + nextBoundary(d.Line(line), off-start)
}

func (d *Document) verticalPos(off, delta int) int {
	line, _, vcol := d.Position(off)
	if d.goal < 0 {
		d.goal = vcol
	}
	target := line + delta
	switch {
	case target < 0:
		return 0
	case target >= d.LineCount():
		return d.buf.Len()
	}
	return d.OffsetAt(target, d.goal)
}

// wordRight skips whitespace, then one run of word or punctuation characters.
// At the end of a line it moves to the start of the next one.
func (d *Document) wordRight(off int) int {
	line := d.lineOf(off)
	start, end := d.lineRange(line)
	if off >= end {
		return d.nextPos(off)
	}
	text := d.Line(line)
	col := off - start
	skipping := classSpace
	forClusters(text, func(s, e int) bool {
		if s < col {
			return true
		}
		c := classOf(text[s:e])
		if skipping == classSpace && c != classSpace {
			skipping = c
		}
		if c != skipping {
			return false
		}
		col = e
		return true
	})
	return start + col
}

// wordLeft is the mirror of wordRight: it lands on the start of a word.
func (d *Document) wordLeft(off int) int {
	line := d.lineOf(off)
	start, _ := d.lineRange(line)
	if off == start {
		return d.prevPos(off)
	}
	text := d.Line(line)
	col := off - start
	var spans [][2]int
	forClusters(text[:col], func(s, e int) bool {
		spans = append(spans, [2]int{s, e})
		return true
	})
	skipping := classSpace
	for i := len(spans) - 1; i >= 0; i-- {
		c := classOf(text[spans[i][0]:spans[i][1]])
		if skipping == classSpace && c != classSpace {
			skipping = c
		}
		if c != skipping {
			break
		}
		col = spans[i][0]
	}
	return start + col
}

func (d *Document) smartHome(off int) int {
	line := d.lineOf(off)
	start, _ := d.lineRange(line)
	text := d.Line(line)
	first := len(text)
	forClusters(text, func(s, e int) bool {
		if classOf(text[s:e]) != classSpace {
			first = s
			return false
		}
		return true
	})
	if off-start == first {
		return start
	}
	return start + first
}
