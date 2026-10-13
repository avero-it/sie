package fs

import (
	"bytes"
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadMissing(t *testing.T) {
	_, err := Store{}.Read(filepath.Join(t.TempDir(), "nope"))
	if !errors.Is(err, iofs.ErrNotExist) {
		t.Errorf("err = %v, want ErrNotExist", err)
	}
}

func TestWriteNewAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	// Edge-case content must survive byte-exactly.
	data := []byte("\xEF\xBB\xBFline\r\nmixed\n\xff\xfe invalid\r")
	if err := (Store{}).Write(path, data); err != nil {
		t.Fatal(err)
	}
	got, err := Store{}.Read(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("round trip: %q, %v", got, err)
	}
	if ok, _ := (Store{}).Exists(path); !ok {
		t.Error("Exists = false after write")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}
}

func TestWritePreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	path := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(path, []byte("old"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := (Store{}).Write(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o750 {
		t.Errorf("mode %v, want 0750", info.Mode().Perm())
	}
}

func TestWriteReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ro.txt")
	if err := os.WriteFile(path, []byte("keep"), 0o444); err != nil {
		t.Fatal(err)
	}
	err := Store{}.Write(path, []byte("new"))
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("err = %v, want ErrReadOnly", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "keep" {
		t.Errorf("read-only file changed: %q", got)
	}
}

func TestWriteFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := (Store{}).Write(link, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Lstat(link); info.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink replaced by a regular file")
	}
	if got, _ := os.ReadFile(target); string(got) != "new" {
		t.Errorf("target content %q", got)
	}
}

func TestWriteDirectory(t *testing.T) {
	if err := (Store{}).Write(t.TempDir(), []byte("x")); err == nil {
		t.Error("writing over a directory succeeded")
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	linked := os.Symlink(filepath.Join(dir, "sub"), filepath.Join(dir, "link")) == nil
	entries, err := Store{}.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Name] = e.Dir
	}
	if dirFlag, ok := got["sub"]; !ok || !dirFlag {
		t.Errorf("sub: %v", got)
	}
	if dirFlag, ok := got[".hidden"]; !ok || dirFlag {
		t.Errorf(".hidden: %v", got)
	}
	if linked && !got["link"] {
		t.Errorf("symlink to dir not a dir: %v", got)
	}
	if _, err := (Store{}).List(filepath.Join(dir, "missing")); err == nil {
		t.Error("listing a missing dir succeeded")
	}
}
