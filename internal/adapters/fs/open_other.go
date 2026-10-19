//go:build !windows

package fs

import "os"

// openShared opens path for reading. On Unix a file that is replaced or
// removed while open stays readable.
func openShared(path string) (*os.File, error) { return os.Open(path) }
