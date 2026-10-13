package recent

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func touch(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAddOrderDedupeAndPersistence(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "state", "recent.json")) // directory created on demand
	if files, err := store.List(); err != nil || files != nil {
		t.Fatalf("empty: %v %v", files, err)
	}
	a, b := touch(t, dir, "a"), touch(t, dir, "b")
	for _, p := range []string{a, b, a} {
		if err := store.Add(p); err != nil {
			t.Fatal(err)
		}
	}
	// A second store (another sie instance) sees the same list.
	files, err := New(store.path).List()
	if err != nil || !slices.Equal(files, []string{a, b}) {
		t.Fatalf("files %v err %v", files, err)
	}
}

func TestMissingFilesDroppedAndLimit(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "recent.json"))
	gone := touch(t, dir, "gone")
	_ = store.Add(gone)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	for i := range Limit + 5 {
		_ = store.Add(touch(t, dir, fmt.Sprintf("f%02d", i)))
	}
	files, _ := store.List()
	if len(files) != Limit || slices.Contains(files, gone) {
		t.Errorf("len %d, contains gone: %v", len(files), slices.Contains(files, gone))
	}
	if files[0] != filepath.Join(dir, fmt.Sprintf("f%02d", Limit+4)) {
		t.Errorf("newest first: %q", files[0])
	}
}

func TestCorruptFileIsReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recent.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := New(path)
	if _, err := store.List(); err == nil {
		t.Error("corrupt file listed without error")
	}
	a := touch(t, dir, "a")
	if err := store.Add(a); err != nil {
		t.Fatal(err)
	}
	if files, err := store.List(); err != nil || !slices.Equal(files, []string{a}) {
		t.Errorf("after replace: %v %v", files, err)
	}
}
