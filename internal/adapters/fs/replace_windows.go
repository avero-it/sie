//go:build windows

package fs

import (
	"fmt"
	"os"
)

// replaceFile renames tmp over target. Windows refuses to replace a file
// that is open, even when it was opened with FILE_SHARE_DELETE. Such a file
// can be renamed and deleted though, so on failure the target is moved
// aside, tmp takes its name, and the old file is deleted: it disappears once
// its last handle (our own view of the document) is closed.
func replaceFile(tmp, target string) error {
	err := os.Rename(tmp, target)
	if err == nil {
		return nil
	}
	aside := fmt.Sprintf("%s.sie-old-%d", target, os.Getpid())
	if os.Rename(target, aside) != nil {
		return err
	}
	if rerr := os.Rename(tmp, target); rerr != nil {
		_ = os.Rename(aside, target) // put the original back
		return rerr
	}
	_ = os.Remove(aside)
	return nil
}
