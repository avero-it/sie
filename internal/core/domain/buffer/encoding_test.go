package buffer

import (
	"bytes"
	"testing"
)

func TestDetectLineEnding(t *testing.T) {
	tests := []struct {
		data string
		def  LineEnding
		want LineEnding
	}{
		{"", LF, LF},
		{"", CRLF, CRLF},
		{"no breaks", CRLF, CRLF},
		{"a\nb\n", CRLF, LF},
		{"a\r\nb\r\n", LF, CRLF},
		{"a\r\nb\r\nc\n", LF, CRLF},
		{"a\r\nb\n", CRLF, CRLF},
		{"a\r\nb\n", LF, LF},
	}
	for _, tt := range tests {
		if got := DetectLineEnding([]byte(tt.data), tt.def); got != tt.want {
			t.Errorf("DetectLineEnding(%q, %v) = %v, want %v", tt.data, tt.def, got, tt.want)
		}
	}
}

func TestSplitBOM(t *testing.T) {
	with := append(UTF8BOM(), "hi"...)
	if has, rest := SplitBOM(with); !has || string(rest) != "hi" {
		t.Errorf("SplitBOM(with) = %v %q", has, rest)
	}
	if has, rest := SplitBOM([]byte("hi")); has || string(rest) != "hi" {
		t.Errorf("SplitBOM(without) = %v %q", has, rest)
	}
	if !bytes.Equal(append(UTF8BOM(), "hi"...), with) {
		t.Error("BOM round-trip mismatch")
	}
}
