//go:build !windows

package fs

import "os"

// replaceFile renames tmp over target. On Unix this works even while target
// is open: readers keep the old contents.
func replaceFile(tmp, target string) error { return os.Rename(tmp, target) }
