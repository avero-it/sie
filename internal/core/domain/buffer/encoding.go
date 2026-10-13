package buffer

import "bytes"

// LineEnding is the line terminator style of a document.
type LineEnding uint8

const (
	LF LineEnding = iota
	CRLF
)

func (e LineEnding) String() string {
	if e == CRLF {
		return "CRLF"
	}
	return "LF"
}

// Bytes returns the terminator bytes.
func (e LineEnding) Bytes() []byte {
	if e == CRLF {
		return []byte("\r\n")
	}
	return []byte("\n")
}

// DetectLineEnding returns the majority line ending in data, or def when data
// has no line breaks or LF and CRLF are tied.
func DetectLineEnding(data []byte, def LineEnding) LineEnding {
	total := bytes.Count(data, []byte{'\n'})
	crlf := bytes.Count(data, []byte("\r\n"))
	lf := total - crlf
	switch {
	case crlf > lf:
		return CRLF
	case lf > crlf:
		return LF
	default:
		return def
	}
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// SplitBOM reports whether data starts with a UTF-8 byte order mark and
// returns data without it. The returned slice aliases data.
func SplitBOM(data []byte) (hasBOM bool, rest []byte) {
	if bytes.HasPrefix(data, utf8BOM) {
		return true, data[len(utf8BOM):]
	}
	return false, data
}

// UTF8BOM returns the UTF-8 byte order mark bytes.
func UTF8BOM() []byte { return bytes.Clone(utf8BOM) }
