//go:build !windows

package clipboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const toolTimeout = 2 * time.Second

// toolClipboard shells out to a clipboard utility (pbcopy, wl-copy, xclip,
// xsel). These are optional: if none is installed there is no native clipboard.
type toolClipboard struct {
	copyCmd, pasteCmd []string
}

func detectNative() native {
	var candidates [][2][]string
	switch {
	case runtime.GOOS == "darwin":
		candidates = append(candidates, [2][]string{{"pbcopy"}, {"pbpaste"}})
	default:
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			candidates = append(candidates, [2][]string{{"wl-copy"}, {"wl-paste", "--no-newline"}})
		}
		if os.Getenv("DISPLAY") != "" {
			candidates = append(candidates,
				[2][]string{{"xclip", "-selection", "clipboard"}, {"xclip", "-selection", "clipboard", "-o"}},
				[2][]string{{"xsel", "--clipboard", "--input"}, {"xsel", "--clipboard", "--output"}})
		}
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0][0]); err == nil {
			return toolClipboard{copyCmd: c[0], pasteCmd: c[1]}
		}
	}
	return nil
}

func (t toolClipboard) copy(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, t.copyCmd[0], t.copyCmd[1:]...)
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", t.copyCmd[0], err)
	}
	return nil
}

func (t toolClipboard) paste() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, t.pasteCmd[0], t.pasteCmd[1:]...).Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", t.pasteCmd[0], err)
	}
	return string(out), nil
}
