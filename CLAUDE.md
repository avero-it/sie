# sie — terminal text editor in Go

`sie` is a modeless terminal text editor (micro/nano-like). Hard requirements:

- **Multiplatform**: Linux, Windows, macOS (amd64 + arm64).
- **Single static binary**, **no C**: every build uses `CGO_ENABLED=0`; every dependency must be pure Go.
- **Hexagonal architecture** (ports & adapters): the core never knows about the terminal, the OS or the filesystem.

This file is the source of truth for humans and agents. If code and this file disagree, fix one of them in the same change.

## Scope

**v0 goals**: single buffer, open/edit/save, undo/redo, selection (keyboard + mouse), clipboard, find/replace, go to line, menu bar, command prompt, status line, horizontal scroll, LF/CRLF preserved, UTF-8 with grapheme-correct cursor movement.

**Non-goals for v0** (future work, don't build them yet): tabs/splits/multiple buffers, soft wrap, syntax highlighting, LSP, plugins, modal (vim) editing, F-key fallbacks for Ctrl+Alt bindings.

## Architecture

```
cmd/sie/                        composition root (cobra CLI: `sie [file]`, `sie version`, `sie config`): loads settings, wires adapters to ports. Only place that knows concrete adapters.
internal/buildinfo/             version, commit, commit/build time, Go version (ldflags first, then embedded VCS info)
internal/core/domain/           pure logic, stdlib only (+ github.com/rivo/uniseg)
  buffer/   piece table, line index, UTF-8/grapheme-aware positions, line-ending detection (LF/CRLF preserved)
  history/  undo/redo as reversible piece-table edits, grouped per command / typing burst
  editor/   Document, Cursor, Selection, Viewport math, search
internal/core/app/              application layer (tcell-free, os-free)
  commands/ generic command registry (Command[E], Arg[E]), inline "name args" parsing
  ui/       prompt Input (grapheme-aware line editing), fuzzy Score
  editor.go Editor (implements EditorService): Dispatch + echo + history, editor-focus keys
  builtin.go the unique list of built-in commands
  menu.go / prompt.go / view.go  menus, prompt modes + completions, layout, hit-testing, ViewModel
internal/core/ports/            interfaces only
  driving:  EditorService (HandleKey, HandleMouse, HandlePaste, Resize, Dispatch, View, Done)
  driven:   FileStore, Clipboard, Keymap   (settings are read once by cmd/sie and passed as plain app.Config values, so no config port)
  values:   KeyEvent, MouseEvent, ViewModel{Runs, Cursor}, Style
internal/adapters/
  tui/        Terminal (owns the tcell screen; New, SetClipboard, Run), event loop, key/mouse/paste normalization, painter + Theme
  keymap/     default modeless keymap (later: loaded from config)
  fs/         os-based FileStore (atomic save: temp file + rename, preserves permissions)
  clipboard/  OSC52 (via Terminal.SetClipboard) + native: Win32 API on Windows, pbcopy / wl-copy / xclip / xsel elsewhere (optional)
  config/     viper: /etc/sie/config.yaml merged with ~/.sie/config.yaml (user wins per key), SIE_* env vars, --config overrides
```

### Dependency rules (non-negotiable)

- `domain` imports only stdlib and `rivo/uniseg`. Never `app`, `ports`, `adapters`, `os`, `tcell`.
- `app` imports `domain` and `ports` only.
- `ports` contains interfaces and the plain value types they need (`KeyEvent`, `MouseEvent`, `ViewModel`, …). No logic.
- `adapters` import `ports` (and `domain`/`app` value types). **Adapters never import each other.**
- Only `cmd/sie` imports concrete adapters.
- Enforced by `internal/archtest` (part of `go test ./...`, no external tools) and by `depguard` in `.golangci.yml`. When the rules change, update both and this section.

### Where does this code go?

| You are writing… | Put it in |
|---|---|
| Text storage, positions, line endings, undo | `internal/core/domain/{buffer,history}` |
| Cursor/selection movement, viewport/scroll math, search | `internal/core/domain/editor` |
| A new user-facing action | a `Command` in `internal/core/app/commands` (+ keymap/menu entries if needed) |
| Menu/prompt/focus behavior, fuzzy matching, what is shown | `internal/core/app/ui` |
| Anything that touches tcell, colors, terminal quirks | `internal/adapters/tui` |
| Key → command bindings | `internal/adapters/keymap` |
| Disk, clipboard, config paths, OS-specific code | the matching adapter in `internal/adapters/` |
| Wiring | `cmd/sie/main.go` |

## TUI model

```
┌ File  Edit  Selection  Search  View  Help ──────────────┐  menu bar             (1 row)
│  12 │ func main() {                                     │
│  13 │     fmt.Println("hi")█                            │  editor               (all remaining rows)
│  14 │ }                                                 │
│ > Save file                                    Ctrl+S   │  command/prompt line  (1 row)
│ main.go  ●  Ln 13, Col 23   UTF-8  LF                    │  status line          (1 row)
└─────────────────────────────────────────────────────────┘
```

### Command registry: one unique list

- Every user-facing action is a `Command{ID, Title, Aliases, Category, Args, Run}` registered **exactly once** in `internal/core/app/commands`. IDs are namespaced: `file.save`, `selection.trimSpaces`, `edit.undo`.
- Keymap (key → ID), menu layout (menu → ordered IDs and separators) and the prompt all refer **to IDs only**. Menu items get their title and shortcut hint from the registry and keymap. Nothing is duplicated.
- Every invocation goes through one `Dispatch(id, args)`, whatever its source: shortcut, menu or prompt.
- Dispatch **echoes** the canonical line into the prompt area (`> Save file   Ctrl+S`) and appends it to prompt history (↑ recalls). This teaches users the shortcuts.
- Adapters never contain command logic. They translate input into a `Dispatch` call or a focus-handler event.
- **Editing primitives are not commands**: arrows (Ctrl = by word, Shift = select), Home/End (Ctrl = document), PgUp/PgDn, Backspace/Del, Enter, Tab, Esc and typed text are handled directly in editor focus and are not echoed. Everything else is a registered command.
- Inline parsing (`Registry.ParseInline`): the longest matching name wins, and a whole-name match counts, so `replace all a b` is Replace all, not Replace with the arguments `all a b`.
- Echo format: `> Title args — result note`, with the shortcut right-aligned. Errors show as `Title args: error`. `Silent` commands (`ui.prompt`, `ui.menu`) are neither echoed nor listed in the prompt.
- **Prompt-only commands** are registered normally but have no key binding and no menu entry, e.g. `edit.deleteToLineEnd` ("Delete to end of line", aliases "delete till end of line" and "kill line"). To reach a command by another phrase, add an alias; never register a second command for the same action.
- Menus are app data (`app.Menu{Title, Items []commandID}`, `"-"` is a separator). `app.New` rejects menus that reference unknown IDs. Commands that take arguments show `…` in menus.
- Unbound rune keys are typed only with no modifiers, Shift only, or Ctrl+Alt together (AltGr on Windows). Plain Alt+letter is ignored.

### Prompt

- **Fuzzy matching** (`ui.Score`) over titles and aliases. With empty input the list shows the 5 most recent invocations (with their args), then all commands by title. A completion popup is drawn just above the prompt line, over the editor, and shows the top matches with their shortcuts. Enter runs the highlighted match.
- **Arguments**: typed inline (`goto 42`, `find foo`), or asked step by step (`Line:`) when missing. Invoking from a shortcut or menu without arguments leads to the same step-by-step prompts.
- The prompt also handles confirmations (unsaved changes on quit, overwrite on save).

### Focus state machine (app layer, unit-testable)

`Editor ⇄ Menu(open menu, highlighted item) ⇄ Prompt(input, completions, highlighted, arg step)`. Esc always returns to Editor.

### Runtime loop (tui adapter)

1. `tcell.PollEvent` runs in its own goroutine and feeds a channel. **A single main goroutine owns all state**, so there are no locks in the core. That goroutine also selects on timers and async results.
2. Each tcell event is normalized to `ports.KeyEvent` / `ports.MouseEvent` and passed to `EditorService`. tcell types never cross into the core.
3. Pending events are drained before each render. A bracketed paste becomes a single `Insert`, which is one undo step.
4. The app builds the **ViewModel**: a list of positioned text `Run`s with a semantic `Style`, plus the cursor. Every layout decision is made in the app (`view.go`): gutter, horizontal clipping of wide glyphs, the selection, the dropdown, completions and right-aligned hints. The painter clears the screen, draws the runs grapheme by grapheme (uniseg widths), places the cursor and calls `Show()`; tcell diffs the frame. App tests render the ViewModel to a text grid, with no terminal needed.
   - **Mouse hit-testing is in the app as well** (`HandleMouse` gets raw cells). It uses the same geometry helpers as the view, so drawn and clickable areas can't drift apart.
5. Viewport, scroll and column↔screen-x math live in the core. The adapter only reports `Resize(w, h)`.
6. `defer screen.Fini()` plus `recover()` always restore the terminal, even on panic.

### Default keymap (v0)

| Key | Command | Key | Command |
|---|---|---|---|
| Ctrl+N | New | Ctrl+F | Find |
| Ctrl+O | Open | Ctrl+R | Replace |
| Ctrl+S | Save | Ctrl+G | Go to line |
| Ctrl+Q | Quit | Ctrl+D | Duplicate line |
| Ctrl+Z / Ctrl+Y | Undo / Redo | Ctrl+K | Delete line |
| Ctrl+X / C / V | Cut / Copy / Paste | Ctrl+L | Select line |
| Ctrl+A | Select all | Ctrl+R | Replace… (interactive) |
| **Ctrl+Alt+P** | Focus prompt | **Ctrl+Alt+M** | Open menus (also: mouse click) |
| Esc | Back to editor (closes menu, prompt or popup) | | |

**Navigation** (editing primitives in `app.navigate`, not commands, not echoed). **Shift** with any of them extends the selection.

| Key | Moves | Key | Moves |
|---|---|---|---|
| ←/→ | character | ↑/↓ | line |
| Ctrl+←/→, Alt+←/→, Alt+B/F | word (Alt+B/F is what macOS Terminal sends for Option+←/→) | Alt+↑/↓ | `jump_lines` lines (config, default 5) |
| Cmd+←/→, Home/End | line start (smart: indent first) / end | Cmd+↑/↓, Ctrl+Home/End | document start/end |
| PgUp/PgDn | one screen | | |

**Cmd** (`ports.ModSuper`) only arrives from terminals that speak the kitty keyboard protocol. tcell enables it automatically on xterm-like terminals (`CSI > 1 u`), and reports Super as `tcell.ModMeta`, which `tui` maps to `ModSuper`, never to Alt. Cmd+letter never types text.

**Interactive replace** (`search.replace`, "Replace…", Ctrl+R, aliases "find and replace" and "query replace"):
- It asks for the search text and the replacement, starts at the selection or cursor, wraps once, and stops before the starting point.
- Each match is selected, and the prompt line asks `y/n/a/q`.
- The whole session is one undo step (`Document.BeginUndoGroup`/`EndUndoGroup`).
- Closing the prompt in any way (Esc, q, a click, the prompt toggle) ends the session through `promptState.onClose`.
- `search.replaceAll` ("Replace all…") has no key binding.

**Convention: Ctrl+Alt+letter is reserved for editor-level functions** (prompt, menu, future ones). There are deliberately no F-key fallbacks.

### Terminal key facts (read before touching the keymap)

- tcell (v2.13) delivers Ctrl+letter in two forms: as control-byte keys `KeySOH..KeySUB` (legacy input) or as `KeyCtrlA..KeyCtrlZ` (numbered from 65; extended key reporting). `tui.normalizeKey` maps both to `ports.Ctrl(letter)`. Ctrl+M/J/I/H are folded to Enter/Enter/Tab/Backspace so every terminal behaves the same way.
- Bracketed paste arrives as `EventPaste` start, key events, `EventPaste` end. The adapter collects these into a single `HandlePaste`.

- Ctrl+Alt+X arrives as ESC + Ctrl byte, which tcell decodes as `KeyCtrlX` with `ModAlt`.
- **Ctrl+Alt+M is the same byte sequence as Alt+Enter**, so never bind Alt+Enter to anything else.
- macOS: Alt reaches the terminal only with "Use Option as Meta" enabled (Terminal.app / iTerm2). The README must say so.
- Windows: Ctrl+Alt equals AltGr and may type a character on some layouts (e.g. German AltGr+M gives `µ`).
- Linux desktops grab some Ctrl+Alt combinations (T, L, Del, F-keys, arrows). Check new Ctrl+Alt bindings against them.
- Never bind Ctrl+M (Enter), Ctrl+I (Tab) or Ctrl+[ (Esc). Avoid Ctrl+H (Backspace on some terminals), Ctrl+J (LF) and Ctrl+Shift combinations.
- Raw mode disables XON/XOFF, so Ctrl+S and Ctrl+Q are usable.
- Free Ctrl keys for future commands: **B, E, P, T, U, W**.
- Alt+B and Alt+F are taken by word moves (readline/macOS Terminal). Plain Alt+letter otherwise does nothing.

## Text model

- Text is stored as UTF-8 bytes in a piece table: original file buffer (read-only) + add buffer + piece list (`domain/buffer`). Each piece caches its `\n` count, which drives `LineStart`/`LineOf`. The flat piece slice is O(pieces) per operation; swap in a balanced tree behind the same API if profiling demands it.
- Only `\n` terminates a line. In CRLF files the `\r` is part of the line's bytes; `Line()` strips it, and the cursor layer treats `\r\n` as one grapheme (uniseg does too).
- The BOM is not stored in the buffer: `SplitBOM` on load, and the document remembers it and re-adds it on save.
- `domain/editor.Document` owns buffer, history, the single `Selection{Anchor, Head}`, line ending, BOM and tab width.
  - **Cursor invariant:** a selection end is never inside a grapheme cluster or between `\r` and `\n`. `SetSelection` snaps backward. Edits snap forward, because an insert can fuse clusters, e.g. "x" typed before a combining mark. `FuzzDocument` checks this.
  - Inserted text has its line breaks normalized to the document's line ending. Enter auto-indents.
  - Undo keys: typing single graphemes coalesces into one step, and so do backspace runs and delete runs. Pastes, newlines, line operations and replace-all are each one step. Any `Move`/`SetSelection` seals the group.
  - `Layout(line, tabWidth)` is the single source of screen geometry: it returns glyphs with column, width and kind (text/tab/control/invalid), and the painter must use it. `Glyph.Display` draws spaces and tabs as blanks. Whitespace and empty-cell markers are view options decided in `app/view.go`:
    - **Invisible characters** (`view.toggleInvisibles`, config `show_invisibles`, default off): a space shows as a faint `␣`, and a tab shows a faint `»` in its first cell, the rest blank. Style: `StyleWhitespace`.
    - **Empty cells** (`view.toggleEmptyCells`, config `show_empty_cells`, default on): every text-area cell holding no character (past the end of the line, and rows below the last line) shows a faint `·`. Style: `StyleEmptyCell`. The gutter stays blank. A selected line break keeps its one selected cell, and the dots start after it.
    - The default theme draws both as bright black + dim. Markers must be one cell wide; prefer East Asian width Neutral, or Latin-1 for font coverage.
    - Control characters (`^X`, 2 cells), zero-width characters (`¤`) and invalid UTF-8 (U+FFFD) are always visible, whatever the toggles.
  - Movement and layout scan the whole line (O(line length)), which is fine for v0. Very long lines (minified files) are a known future optimization.
  - Case-insensitive search uses simple folding over `len(query)` bytes, so matches whose folded form has a different byte length are missed.
- Undo (`domain/history`) stores reversible `Edit{Off, Removed, Inserted}` groups with caller state (cursor and selection). The caller groups typing with a coalescing key and calls `Seal()` on cursor moves. `Modified()` is derived from the saved position in the undo stack.
- Positions in the domain are byte offsets. The cursor moves by **grapheme cluster** (uniseg). Layout uses **display width**: CJK and emoji take 2 cells, combining marks take 0, and tabs expand to the configured width.
- Line endings are detected on load (LF / CRLF) and preserved on save. Mixed files keep the majority style for new lines.
- A UTF-8 BOM is preserved if present. Invalid UTF-8 must never corrupt the file: bytes round-trip unchanged.

## Commands

```sh
go build ./...
go test ./...
go test -race ./...
go test -run='^$' -fuzz=FuzzPieceTable -fuzztime=30s ./internal/core/domain/buffer
go test -run='^$' -fuzz=FuzzDocument -fuzztime=30s ./internal/core/domain/editor
go vet ./...
golangci-lint run

# release build of one target (build info: see internal/buildinfo)
P=github.com/avero-it/sie/internal/buildinfo
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X $P.version=v0.1.0 -X $P.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o sie ./cmd/sie

# cross-compile matrix — must all pass, cgo off
for os in linux windows darwin; do for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -o /dev/null ./cmd/sie || echo "FAIL $os/$arch"
done; done

# no cgo anywhere in the dependency graph (must print nothing)
CGO_ENABLED=0 go list -deps -f '{{if .CgoFiles}}{{.ImportPath}}{{end}}' ./...
```

## Conventions

- Go 1.26, standard `gofmt`/`goimports`. Packages are small and named for what they hold, not `utils`.
- Interfaces live in `ports` (consumer side), are small, and are only introduced when there is a second implementation or a test fake.
- Errors are wrapped with `%w` and carry context. No panics in `domain` or `app`. Invalid input returns errors.
- No global mutable state. Everything is constructed in `cmd/sie` and passed in.
- `context.Context` only at app/adapter boundaries (I/O), never in the domain.
- Tests are table-driven. Use `testing/fstest` or in-memory fakes for ports. Fuzz the piece table against a naive `[]byte` model. Use `tcell.SimulationScreen` golden tests for rendering (`-update` flag regenerates goldens).
- **Dependencies are vendored.** `vendor/` is committed, and the go command builds from it automatically, so builds need no network. After any change to `go.mod` (`go get`, version bumps), run `go mod tidy && go mod vendor` and commit `vendor/` in the same commit. `go mod verify` checks the module cache against `go.sum`. Never edit files under `vendor/`. archtest and gofmt checks skip it (`gofmt -l $(git ls-files '*.go' | grep -v '^vendor/')`).
- **New dependencies need justification** in the PR/commit message: pure Go, maintained, and not trivially replaceable by the stdlib. Current allow-list: `github.com/gdamore/tcell/v2` v2.13.10 (adapters/tui only; `cmd/sie` gets the screen through `tui.New`), `github.com/rivo/uniseg`, `golang.org/x/sys` (adapters only, for the Windows clipboard; already a tcell dependency), `github.com/spf13/viper` v1.21.0 (adapters/config only; requested for config files), `github.com/spf13/cobra` v1.10.2 (cmd/sie only; requested for the CLI). archtest enforces where tcell, viper and cobra may be imported. Pin exact versions that are at least two weeks old.

## Definition of done

1. `go vet`, `golangci-lint run` and `go test -race ./...` pass.
2. The cross-compile matrix passes, and the cgo check prints nothing.
3. Dependency rules hold (no forbidden imports in `internal/core`).
4. New commands are registered once and reachable from the prompt; bindings and menu entries reference the ID.
5. CLAUDE.md is updated if architecture, keymap or conventions changed.

## Subagents (`.claude/agents/`)

| Agent | Use it for |
|---|---|
| `architect` | Reviewing a change or design for boundary violations, port design, command-registry rules, new dependencies. Read-only. |
| `core-engineer` | Anything under `internal/core/**`: piece table, undo, cursor, viewport, command registry, focus/menu/prompt models, fuzzy matcher. |
| `tui-engineer` | `internal/adapters/tui` and `internal/adapters/keymap`: event loop, painting, mouse hit-testing, terminal quirks, golden tests. |
| `platform-qa` | Cross-compilation, cgo checks, fs/clipboard/config adapters, file edge cases (CRLF, BOM, large files, permissions), CI and release. |
