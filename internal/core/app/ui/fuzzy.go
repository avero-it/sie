package ui

import (
	"strings"
	"unicode"
)

// Score rates how well query matches candidate, case-insensitively. Every
// query rune must appear in candidate in order (a subsequence); ok is false
// otherwise. Higher is better: a prefix match beats word-start matches,
// which beat scattered ones; consecutive runs earn a bonus.
func Score(query, candidate string) (score int, ok bool) {
	q := []rune(strings.ToLower(strings.TrimSpace(query)))
	if len(q) == 0 {
		return 0, true
	}
	c := []rune(strings.ToLower(candidate))
	qi, last := 0, -2
	for ci, r := range c {
		if qi == len(q) {
			break
		}
		if r != q[qi] {
			continue
		}
		s := 1
		if ci == 0 || !isWordRune(c[ci-1]) {
			s += 8
		}
		if last == ci-1 {
			s += 4
		}
		score += s
		last = ci
		qi++
	}
	if qi < len(q) {
		return 0, false
	}
	if strings.HasPrefix(string(c), string(q)) {
		score += 50
	}
	return score - len(c)/10, true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
