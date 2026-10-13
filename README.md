# sie

A small, modeless terminal text editor written in Go. It ships as a single static binary for Linux, Windows and macOS, with no C dependencies.

```
┌ File  Edit  Selection  Search  View  Help ──────────────┐
│  1 package main                                         │
│  2                                                      │
│  3 func main() {█                                       │
│ > Save file — wrote 3 lines                     Ctrl+S  │
│ main.go   Ln 3, Col 14   UTF-8   LF                     │
└─────────────────────────────────────────────────────────┘
```

## Install

```sh
go install github.com/avero-it/sie/cmd/sie@latest
```

Or build it yourself. The `-X` flags record the version and build time that `sie version` shows:

```sh
P=github.com/avero-it/sie/internal/buildinfo
CGO_ENABLED=0 go build -trimpath \
  -ldflags="-s -w -X $P.version=v0.1.0 -X $P.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o sie ./cmd/sie
```

## Use

```sh
sie [file]
```

There are three ways to run every command:

1. **Shortcuts**: listed below.
2. **Menus**: `Ctrl+Alt+M`, or click the menu bar.
3. **Command prompt**: `Ctrl+Alt+P`, then type part of a command's name, such as `trim sel`, `goto 42` or `find foo`. With an empty prompt, your recent commands are listed first.

Whichever way you run a command, its name and shortcut appear in the prompt line.

| Key | Command | Key | Command |
|---|---|---|---|
| Ctrl+N | New | Ctrl+F | Find |
| Ctrl+O | Open | Ctrl+R | Replace all |
| Ctrl+S | Save | Ctrl+G | Go to line |
| Ctrl+Q | Quit | Ctrl+D | Duplicate line |
| Ctrl+Z / Ctrl+Y | Undo / Redo | Ctrl+K | Delete line |
| Ctrl+X / C / V | Cut / Copy / Paste | Ctrl+L | Select line |
| Ctrl+A | Select all | Shift + arrows/Home/End | Select |
| **Ctrl+Alt+P** | Command prompt | **Ctrl+Alt+M** | Menus |
| Esc | Back to the editor | Ctrl + arrows | Move by word |

The mouse works too: click to place the cursor, drag or Shift+click to select, and use the wheel to scroll.

### Showing whitespace

- **Empty cells** (cells after the end of a line, and rows below the last line) show a faint `·` by default.
- **Invisible characters** can be shown too: spaces as a faint `␣`, and tabs as a faint `»`.

Toggle either one from the **View** menu or the prompt (`whitespace`, `empty cells`), or set it in the config file.

## Configuration

sie reads `/etc/sie/config.yaml` and then `~/.sie/config.yaml`. Each key in the user file overrides the same key in the system file. `SIE_*` environment variables override both, and `--config <file>` reads only that file.

```yaml
tab_width: 4            # 1–16
show_invisibles: false  # mark spaces (␣) and tabs (»)
show_empty_cells: true  # mark cells holding no character (·)
line_ending: lf         # for new files: lf or crlf
```

`sie config` prints the effective settings and which files they came from. `sie version` prints the version, commit, commit and build time, Go version and platform.

### Terminal notes

- **macOS:** `Ctrl+Alt` shortcuts need the Option key to act as Meta. In Terminal.app, go to *Settings → Profiles → Keyboard* and enable **Use Option as Meta key**. In iTerm2, go to *Settings → Profiles → Keys* and set the **Left Option key** to **Esc+**.
- **Windows:** `Ctrl+Alt` is AltGr on many keyboard layouts. If an `AltGr` combination types a character, sie inserts that character, unless that exact combination is bound to a command.
- **Clipboard:** copy goes to the system clipboard through the terminal (OSC 52, which also works over SSH) and through the native clipboard: Win32 on Windows, `pbcopy` on macOS, and `wl-copy`, `xclip` or `xsel` on Linux. Pasting from other applications works through the native clipboard or through your terminal's own paste.

## Development

See [CLAUDE.md](CLAUDE.md) for the architecture (hexagonal: `internal/core` is pure; terminal, filesystem and clipboard code live in `internal/adapters`), conventions and the full check list.

```sh
go test -race ./...
```
