# sie

**sie** stands for **siMPLE eDITOR**: the lowercase letters spell its name.

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
| Ctrl+E | Open recent (fuzzy search) | Ctrl+W | Close file |
| Ctrl+T | Validate (JSON, YAML, TOML, XML) | | |
| Ctrl+O | Open | Ctrl+R | Replace all |
| Ctrl+S | Save | Ctrl+G | Go to line |
| Ctrl+Q | Quit | Ctrl+D | Duplicate line |
| Ctrl+Z / Ctrl+Y | Undo / Redo | Ctrl+K | Delete line |
| Ctrl+X / C / V | Cut / Copy / Paste | Ctrl+L | Select line |
| Ctrl+A | Select all | Ctrl+R | Replace… (asks y/n/a/q per match) |
| **Ctrl+Alt+P** | Command prompt | **Ctrl+Alt+M** | Menus |
| Esc | Back to the editor | | |

Moving the cursor (hold **Shift** with any of these to select):

| Key | Moves | Key | Moves |
|---|---|---|---|
| Alt+←/→ (or Ctrl+←/→) | by word | Alt+↑/↓ | 5 lines (`jump_lines`) |
| Cmd+←/→ or Home/End | line start/end | Cmd+↑/↓ or Ctrl+Home/End | file start/end |

The mouse works too: click to place the cursor, drag or Shift+click to select, and use the wheel to scroll.

### Opening and saving files

**Ctrl+O** (Open) and **Save as** show a file browser in the prompt line:
- Type a path. Entries of the directory that start with what you typed are listed above, directories first.
- **Tab** completes. **↑/↓** or a click selects. **Enter** goes into a directory or opens a file. **Backspace** right after a `/` goes up a level.
- `~` is your home directory.
- If nothing is selected, Enter takes exactly what you typed, so a name that doesn't exist yet creates a new file.

**Ctrl+E** (Open recent) lists the files you opened or saved most recently, newest first. They're kept in `~/.sie/recent.json` (30 files), shared by all sie windows. Type a few letters of the path to filter, then press Enter to open the top match.

### Validating files

**Ctrl+T** (Validate) checks JSON, YAML, TOML and XML files. The type is taken from the file name or from *Set language…*.
- **A valid file** shows `✓ valid YAML`.
- **A problem** moves the cursor onto it, with a description like `Validate: line 3, col 21: invalid character ']' looking for beginning of value`.
- **YAML** also reports duplicate keys.

Saving also validates these file types. If the file has a problem, sie asks before writing: **v** shows the problem, **s** saves anyway, and **Esc** cancels. Turn this off with *View → Validate on save* or `validate_on_save: false`.

### Showing whitespace

- **Empty cells** (cells after the end of a line, and rows below the last line) show a faint `·` by default.
- **Invisible characters** can be shown too: spaces as a faint `␣`, and tabs as a faint `»`.

Toggle either one from the **View** menu or the prompt (`whitespace`, `empty cells`), or set it in the config file.

### Syntax highlighting

Files are colored by type: Go, JSON and YAML, plus every other language [chroma](https://github.com/alecthomas/chroma) knows. The type comes from the file name and is shown in the status line.
- To pick it yourself, use **View → Set language…**, or `language yaml` in the prompt. `language plain` turns coloring off for that file.
- **View → Syntax highlighting** turns coloring on or off.
- Files larger than `syntax_max_size` stay plain.

Coloring runs in the background, so typing never waits for it. Right after an edit, the edited token may briefly show without color.

## Configuration

sie reads `/etc/sie/config.yaml` and then `~/.sie/config.yaml`. Each key in the user file overrides the same key in the system file. `SIE_*` environment variables override both, and `--config <file>` reads only that file.

```yaml
tab_width: 4            # 1–16
show_invisibles: false  # mark spaces (␣) and tabs (»)
show_empty_cells: true  # mark cells holding no character (·)
line_ending: lf         # for new files: lf or crlf
jump_lines: 5           # lines moved by Alt+↑/↓ (1–100)
syntax_highlighting: true
syntax_max_size: 2MB    # larger files are not colored
validate_on_save: true  # check JSON/YAML/TOML/XML before saving
```

`sie config` prints the effective settings and which files they came from. `sie version` prints the version, commit, commit and build time, Go version and platform.

### Terminal notes

- **macOS:** `Ctrl+Alt` shortcuts need the Option key to act as Meta. In Terminal.app, go to *Settings → Profiles → Keyboard* and enable **Use Option as Meta key**. In iTerm2, go to *Settings → Profiles → Keys* and set the **Left Option key** to **Esc+**.
- **Cmd (⌘) shortcuts** only work in terminals that report Cmd to programs through the kitty keyboard protocol, which sie turns on automatically: kitty, WezTerm, Ghostty and other terminals that implement it. **Terminal.app does not report Cmd**, so use Home/End there (Fn+←/→ on Mac keyboards). In iTerm2 **don't** use the "Natural Text Editing" key preset as-is: it sends ⌘← as Ctrl+A, which is *Select all* in sie.
- **Option (⌥)+←/→** works out of the box in Terminal.app (it sends Alt+B/Alt+F, which sie treats as word moves). Shift+Option may not reach sie there without a custom key mapping.
- **Windows:** `Ctrl+Alt` is AltGr on many keyboard layouts. If an `AltGr` combination types a character, sie inserts that character, unless that exact combination is bound to a command.
- **Clipboard:** copy goes to the system clipboard through the terminal (OSC 52, which also works over SSH) and through the native clipboard: Win32 on Windows, `pbcopy` on macOS, and `wl-copy`, `xclip` or `xsel` on Linux. Pasting from other applications works through the native clipboard or through your terminal's own paste.

## Development

See [CLAUDE.md](CLAUDE.md) for the architecture (hexagonal: `internal/core` is pure; terminal, filesystem and clipboard code live in `internal/adapters`), conventions and the full check list.

```sh
go test -race ./...
```
