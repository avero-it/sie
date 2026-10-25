# sie

**sie** stands for **siMPLE eDITOR**: the lowercase letters spell its name.

Website: <https://avero-it.github.io/sie/> · Changelog: <https://avero-it.github.io/sie/changelog.html>.

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

**Homebrew** (macOS, Linux):

```sh
brew install --cask avero-it/tap/sie
```

**Scoop** (Windows):

```powershell
scoop bucket add avero-it https://github.com/avero-it/scoop-bucket
scoop install sie
```

**Debian, Ubuntu** (apt repository, updated with every release; amd64 and arm64):

```sh
sudo curl -fsSL https://avero-it.github.io/sie/sie.gpg -o /usr/share/keyrings/sie.gpg
echo "deb [signed-by=/usr/share/keyrings/sie.gpg] https://avero-it.github.io/sie stable main" | sudo tee /etc/apt/sources.list.d/sie.list
sudo apt update && sudo apt install sie
```

After that, `sudo apt upgrade` keeps sie current.

**Other Linux packages:** download the package for your system from the [releases page](https://github.com/avero-it/sie/releases), then:

```sh
sudo apt install ./sie_<version>_linux_amd64.deb             # Debian, Ubuntu
sudo dnf install ./sie_<version>_linux_amd64.rpm             # Fedora, RHEL
sudo apk add --allow-untrusted ./sie_<version>_linux_amd64.apk # Alpine
```

**Manually:** download the archive for your platform from the [releases page](https://github.com/avero-it/sie/releases), unpack it and put `sie` (`sie.exe` on Windows) somewhere on your `PATH`. Each release lists SHA-256 checksums in `checksums.txt`.

On macOS, a binary downloaded with a browser is quarantined (the Homebrew cask takes care of this for you). Allow it once with:

```sh
xattr -d com.apple.quarantine ./sie
```

With Go installed:

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

Whichever way you run a command, its name and shortcut appear in the prompt line. Every command has a shortcut: the rarer ones are chords, **Ctrl+P** followed by a letter (below).

| Key | Command | Key | Command |
|---|---|---|---|
| Ctrl+N | New | Ctrl+F | Find |
| Ctrl+E | Open recent (fuzzy search) | Ctrl+W | Close file |
| Ctrl+T | Validate (JSON, YAML, TOML, XML) | Ctrl+U | Column selection (on/off) |
| Ctrl+O | Open | Ctrl+R | Replace… (asks y/n/a/q per match) |
| Ctrl+S | Save | Ctrl+G | Go to line |
| Ctrl+Q | Quit | Ctrl+D | Duplicate line |
| Ctrl+Z / Ctrl+Y | Undo / Redo | Ctrl+K | Delete line |
| Ctrl+X / C / V | Cut / Copy / Paste | Ctrl+L | Select line |
| Ctrl+A | Select all | Ctrl+/ | Toggle comment |
| **Ctrl+Alt+P** | Command prompt | **Ctrl+Alt+M** | Menus |
| Esc | Back to the editor | | |

Chords (press **Ctrl+P**, release, then the letter; Esc cancels):

| Chord | Command | Chord | Command |
|---|---|---|---|
| Ctrl+P A | Save as | Ctrl+P I | Invisible characters |
| Ctrl+P K | Delete to end of line | Ctrl+P E | Empty cells |
| Ctrl+P L | Trim leading spaces | Ctrl+P C | Click ripple |
| Ctrl+P T | Trim trailing spaces | Ctrl+P G | Indent guide |
| Ctrl+P P | Align to previous line | Ctrl+P S | Syntax highlighting |
| Ctrl+P N | Align to next line | Ctrl+P Y | Set language… |
| Ctrl+P F / B | Find next / previous | Ctrl+P W | Validate on save |
| Ctrl+P R | Replace all… | Ctrl+P H | About |
| Ctrl+P U / J | Move line up / down | Ctrl+P M | Toggle comment |
| Ctrl+P V / Q | Select next / previous occurrence | Ctrl+P Z | Wrap lines |
| Ctrl+P X | Split view (two stacked panes) | Ctrl+P 2 | Switch pane |
| Ctrl+P O | Column ruler | Ctrl+P D | Detach MCP |

**Ctrl+/** is the same as Ctrl+P M. On an Italian keyboard that is Ctrl+Shift+7. Some terminals don't send it (macOS Terminal.app may not), so use the chord there. Toggle comment needs a known language (see Set language…).

Moving the cursor (hold **Shift** with any of these to select):

| Key | Moves | Key | Moves |
|---|---|---|---|
| Alt+←/→ (or Ctrl+←/→) | by word | Alt+↑/↓ | 5 lines (`jump_lines`) |
| Cmd+←/→ or Home/End | line start/end | Cmd+↑/↓ or Ctrl+Home/End | file start/end |

Editing keys:

| Key | Does |
|---|---|
| Tab | Insert a tab. With several lines selected, indent them all |
| Shift+Tab | Delete the tab or spaces left of the cursor; if there are none, outdent the line (a tab, or up to a tab stop of spaces). With several lines selected, outdent them all |
| Alt+Backspace / Alt+Delete | Delete the word before / after the cursor |

The mouse works too: click to place the cursor, drag or Shift+click to select, and use the wheel to scroll.

**Multiple cursors:** Alt+click adds a cursor (Alt+click on a cursor removes it). Typing, Enter, Tab, Backspace, Delete, Paste and the arrow keys then act on every cursor, and Undo restores them all. A plain click, Esc or any other command goes back to a single cursor.

**Selecting a block (rectangle):**
- **Alt+drag** draws a rectangle from corner to corner. Or turn on **Column selection** (**Ctrl+U**, also in the Selection menu): then a plain drag draws a rectangle, and **Shift+←/→/↑/↓** grow one from the keyboard. The status line shows `COLUMN` while the mode is on.
- A rectangle is one selection per line, so typing, Backspace, Delete and Undo work on all lines at once. Typing with a zero-width rectangle (a vertical line of cursors) inserts a column of text. Lines shorter than the rectangle get a cursor at their end.
- Columns are screen columns: a tab or a wide character counts by its width.
- **Copy and Cut** take the text of every selection, one per line. **Paste** puts one line on each cursor when the line count matches the cursor count, otherwise the whole text goes to every cursor. Esc goes back to one cursor.

### Opening and saving files

**Ctrl+O** (Open) and **Save as** show a file browser in the prompt line:
- Type a path. Entries of the directory that start with what you typed are listed above, directories first.
- **Tab** completes. **↑/↓** or a click selects. **Enter** goes into a directory or opens a file. **Backspace** right after a `/` goes up a level.
- `~` is your home directory.
- If nothing is selected, Enter takes exactly what you typed, so a name that doesn't exist yet creates a new file.

**Ctrl+E** (Open recent) lists the files you opened or saved most recently, newest first. They're kept in `~/.sie/recent.json` (30 files), shared by all sie windows. Type a few letters of the path to filter, then press Enter to open the top match.

### Large files

Files over `large_file_size` (32 MB by default) are not loaded into memory. They stay on disk and sie reads the part you are looking at, so a 1 GB log opens in a fraction of a second and uses a few megabytes of RAM. Editing, going to a line, finding and saving work as usual.
- Validation, and syntax colors above `syntax_max_size`, are off for these files. Copying more than 64 MB at once is refused.
- If the file is changed by another program while you have it open, sie asks you to reload it or quit, because the text on screen can no longer be trusted.
- A change too big to undo (more than 256 MB, like deleting a gigabyte) is applied, and the undo history is cleared.
- Very long lines (minified files) are still slow.

### Validating files

**Ctrl+T** (Validate) checks JSON, YAML, TOML and XML files. The type is taken from the file name or from *Set language…*.
- **A valid file** shows `✓ valid YAML`.
- **A problem** moves the cursor onto it, with a description like `Validate: line 3, col 21: invalid character ']' looking for beginning of value`.
- **YAML** also reports duplicate keys.

Saving also validates these file types. If the file has a problem, sie asks before writing: **v** shows the problem, **s** saves anyway, and **Esc** cancels. Turn this off with *View → Validate on save* or `validate_on_save: false`.

### Working with an AI agent (MCP)

`sie mcp` lets an agent such as Claude Code work on the buffer you have open in sie, including text you have not saved. You both edit the same text, and you see every change as it happens. The agent's own file tools stay in charge of everything else.

Register it once:

```sh
claude mcp add sie -- sie mcp
```

Then open a file with `sie file.go` and ask Claude to work on it or on your selection. To keep it using the right tools, add this to your project's `CLAUDE.md`:

```
When the user asks to work on the file open in sie, on their selection
or on unsaved changes, use the mcp__sie__* tools. For everything else
(multiple files, refactoring, files not open) use the native tools.
```

- **You decide who gets in.** The first time the agent asks, the prompt line shows `Claude Code wants to control this buffer`: **y** allows it, **n** refuses, **a** allows it for as long as this sie runs. While an agent is attached the status line shows `● MCP`. *Help → Detach MCP* (or **Ctrl+P D**) takes the buffer back, and it also forgets an *always*.
- **Its edits are yours to review.** An edit goes into the open buffer and marks it modified. One edit is one undo step, so **Ctrl+Z** takes it back. The agent can't save, quit or open files, so you save with **Ctrl+S** when you are happy.
- **It never overwrites what you just typed.** Every edit names the revision of the text the agent read. If you typed in between, the edit is refused and the agent reads again.
- **Only you can reach it.** Each running sie listens on `~/.sie/sock/<pid>.sock` (directory mode 0700, socket 0600), so only your user can connect. There is no network port. Set `mcp: false` (or `SIE_MCP=false`) to turn the socket off.
- **Unix only for now:** Linux and macOS. On Windows the socket is not opened (the `mcp` setting has no effect), `sie mcp` says it is not available, and *Detach MCP* does nothing.

With several sie running, the agent attaches to the one you used last, or to the one it picks with `list_sessions`. Text that isn't valid UTF-8 is handed to the agent with `\xNN` escapes so it can't be damaged by an edit.

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
click_ripple: false     # a wave animation on every click in the text
indent_guide: false     # briefly show a vertical guide when a line's indent moves
wrap: false             # wrap long lines instead of scrolling sideways
split: false            # two stacked panes onto the same document
column_ruler: true      # column numbers on the first text row (not while wrapping)
line_ending: lf         # for new files: lf or crlf
jump_lines: 5           # lines moved by Alt+↑/↓ (1–100)
syntax_highlighting: true
syntax_max_size: 2MB    # larger files are not colored
validate_on_save: true  # check JSON/YAML/TOML/XML before saving
mcp: true               # open the local socket that `sie mcp` talks to (SIE_MCP=false turns it off)
large_file_size: 32MB   # larger files stay on disk and are read as needed
```

`sie config` prints the effective settings and which files they came from. `sie version` prints the version, commit, commit and build time, Go version and platform.

### Terminal notes

- **macOS:** `Ctrl+Alt` shortcuts need the Option key to act as Meta. In Terminal.app, go to *Settings → Profiles → Keyboard* and enable **Use Option as Meta key**. In iTerm2, go to *Settings → Profiles → Keys* and set the **Left Option key** to **Esc+**.
- **Cmd (⌘) shortcuts** only work in terminals that report Cmd to programs through the kitty keyboard protocol, which sie turns on automatically: kitty, WezTerm, Ghostty and other terminals that implement it. **Terminal.app does not report Cmd**, so use Home/End there (Fn+←/→ on Mac keyboards). In iTerm2 **don't** use the "Natural Text Editing" key preset as-is: it sends ⌘← as Ctrl+A, which is *Select all* in sie.
- **Option (⌥)+←/→** works out of the box in Terminal.app (it sends Alt+B/Alt+F, which sie treats as word moves). Shift+Option may not reach sie there without a custom key mapping.
- **Alt+click** and **Alt+drag** need the terminal to pass Alt with mouse clicks. On macOS enable *Use Option as Meta key* (see above); some Linux desktops use Alt+click to move windows, so change that shortcut in the desktop settings if Alt+click does nothing in sie.
- **Windows:** `Ctrl+Alt` is AltGr on many keyboard layouts. If an `AltGr` combination types a character, sie inserts that character, unless that exact combination is bound to a command.
- **Clipboard:** copy goes to the system clipboard through the terminal (OSC 52, which also works over SSH) and through the native clipboard: Win32 on Windows, `pbcopy` on macOS, and `wl-copy`, `xclip` or `xsel` on Linux. Pasting from other applications works through the native clipboard or through your terminal's own paste.

## License

[MIT](LICENSE) © 2026 Simone Baglioni

## Development

See [CLAUDE.md](CLAUDE.md) for the architecture (hexagonal: `internal/core` is pure; terminal, filesystem and clipboard code live in `internal/adapters`), conventions and the full check list.

```sh
go test -race ./...
```
