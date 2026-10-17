# sie — terminal text editor in Go

`sie` is a modeless terminal text editor (micro/nano-like). Hard requirements:

- **Multiplatform**: Linux, Windows, macOS (amd64 + arm64).
- **Single static binary**, **no C**: every build uses `CGO_ENABLED=0`; every dependency must be pure Go.
- **Hexagonal architecture** (ports & adapters): the core never knows about the terminal, the OS or the filesystem.

This file is the source of truth for humans and agents. If code and this file disagree, fix one of them in the same change.

## Scope

**v0 goals**: single buffer, open/edit/save, undo/redo, selection (keyboard + mouse), multiple cursors (Alt+click), rectangular selection (Alt+drag, column mode), clipboard, find/replace, go to line, menu bar, command prompt, status line, horizontal scroll, LF/CRLF preserved, UTF-8 with grapheme-correct cursor movement.

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
  driving:  EditorService (HandleKey, HandleMouse, HandlePaste, Resize, Dispatch, View, Done, Poll)
  driven:   FileStore (Read, Write, Exists, List), RecentFiles, Validator, Clipboard, Keymap, Highlighter (async: Highlight job → Result)   (settings are read once by cmd/sie and passed as plain app.Config values, so no config port)
  values:   KeyEvent, MouseEvent, ViewModel{Runs, Tints, Cursor, Animate}, Style, Token/TokenClass, HighlightJob/Result
internal/adapters/
  tui/        Terminal (owns the tcell screen; New, SetClipboard, Run), event loop, key/mouse/paste normalization, painter + Theme
  keymap/     default modeless keymap (later: loaded from config)
  recent/     recent files in ~/.sie/recent.json (re-read and merged on every Add, atomic writes, 30 entries)
  validate/   Validator: JSON (encoding/json), YAML (goccy/go-yaml), TOML (go-toml/v2), XML (encoding/xml)
  syntax/     chroma highlighter: background worker (newest job wins), chroma token types → core TokenClass
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
│ > Save file                                    Ctrl+S   │  command/prompt line  (1 row, gray bar)
│ sie v0.2.1 - main.go ●      Ln 13, Col 23   UTF-8   LF  │  status line          (1 row)
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
- **On/off options** set `Command.Checked` (it reports whether the option is on); they are not separate show and hide commands.
  - Title them after the thing they enable ("Empty cells", "Invisible characters"), and keep old phrasings as aliases.
  - Every menu item and prompt completion has a 2-cell tick column: `✓ ` (U+2713, width Neutral) when the option is on, otherwise blank, so labels line up.
  - The echo carries the same column: `> ✓ Empty cells — shown`, `>   Empty cells — hidden`.
- Echo format: `> Title args — result note`, with the shortcut right-aligned. Errors show as `Title args: error`. `Silent` commands (`ui.prompt`, `ui.menu`) are neither echoed nor listed in the prompt.
- **Prompt-only commands** are registered normally but have no key binding and no menu entry, e.g. `edit.deleteToLineEnd` ("Delete to end of line", aliases "delete till end of line" and "kill line"). To reach a command by another phrase, add an alias; never register a second command for the same action.
- Menus are app data (`app.Menu{Title, Items []commandID}`, `"-"` is a separator). `app.New` rejects menus that reference unknown IDs. Commands that take arguments show `…` in menus.
- Unbound rune keys are typed only with no modifiers, Shift only, or Ctrl+Alt together (AltGr on Windows). Plain Alt+letter is ignored.

### Syntax highlighting

- **Engine:** chroma (`internal/adapters/syntax`, the only package allowed to import it, enforced by archtest). It supports every chroma language. Go, JSON and YAML are tested.
- **Detection:** by file name (`Highlighter.Detect`), redone on open and Save as. `go.mod`, `go.sum` and `go.work` stay plain, because chroma would call them AMPL. "Set language…" (`view.setLanguage`, aliases `language`, `syntax`, `filetype`) overrides detection; `plain` turns coloring off for this document, and the override survives Save as. The status line shows the language (`Go`, `Plain`, or `Go (too large)`).
- **Asynchronous:** after every input event the app calls `syncHighlight`. If the document version changed, it sends a `HighlightJob` with a copy of the text. The worker tokenizes only the newest pending job, stores the result and calls `Terminal.Wake`. The loop then draws, and every draw calls `EditorService.Poll` first, which collects the result.
  - Job IDs are `epoch<<40 | document version`. The epoch changes when the document or language changes, so late results for an older document are discarded.
- **Between results:** the kept tokens are mapped through `Document.ChangesSince` (a change log covering edits, undo and redo). Tokens before an edit stay, tokens after it shift, and tokens touched by it are dropped and drawn plain until the next result. Typing latency doesn't depend on file size; colors settle about one scan later (about 50 ms for 130 KB of Go).
- **Limits and toggle:** documents over `syntax_max_size` (default `2MB`) are never sent to the highlighter. "Syntax highlighting" (`view.toggleSyntax`, ✓ option, config `syntax_highlighting`, default on) turns it off.
- **Classes and precedence:** keyword, string, number, comment, constant, key, type, builtin and meta, mapped in `syntax.classify`. Precedence: whitespace markers > selection > control/invalid glyphs > syntax class.

### File dialog (Open, Save as)

- Arguments with `Kind: commands.ArgPath` make the prompt a file browser (`app/files.go`). Any future command that takes a path gets it for free.
- The input is a path. The text after the last separator filters the listed directory by prefix, ignoring case. `/` is accepted on every OS, and dotfiles are always listed. The popup shows up to 12 rows: `../` first (except at the root), then directories, then files. An exact name match is listed first, and a listing error shows as an unselectable line.
- **Keys:**
  - Tab completes the longest common prefix, or the single match (a directory gets a trailing separator).
  - ↑/↓ or a click highlights an entry. Nothing is highlighted at first.
  - Enter acts on the highlighted entry: a directory is entered, a file is chosen. With no highlight, Enter takes the typed text (entering it if it's a directory), so a partial name creates a new file rather than opening the first match.
  - Backspace right after a separator goes up one directory.
  - A single click acts.
- **Paths:**
  - Browsing starts in the current file's directory.
  - Paths are shown relative to `Config.WorkDir` when inside it, else as `~/…` (`Config.HomeDir`), else absolute.
  - `~` is expanded in `cmdOpen`/`cmdSaveAs`, so it also works inline: `open ~/notes.txt`.
  - The core only uses `path/filepath`. Listing goes through `FileStore.List`, and `cmd/sie` supplies the working and home directories (`os.Getwd`, `os.UserHomeDir`).

### Open recent

- `file.openRecent` ("Open recent…", **Ctrl+E**, aliases `recent` and `recent files`) uses `commands.ArgRecent`. The prompt lists the recent files most recent first: the current file and files that no longer exist are hidden. Typing fuzzy-filters by the displayed path (`ui.Score`), and the best match is highlighted, so Enter opens it. Tab copies the highlighted path, and a click opens it.
- Inline (`recent todo`) opens the best fuzzy match.
- Every successful open of an existing file and every save records the file (`recordRecent`, absolute path). Failures to record are ignored, because the list must never get in the way of editing.

### Validate and Close file

- **Validate** (`edit.validate`, **Ctrl+T**, Edit menu, aliases `check` and `lint`):
  - It checks the document as its language: the detected one, or the one chosen with "Set language…". The `ports.Validator` reports the first problem with a 1-based line and a character column (`Col 0` means line only, `Line 0` means position unknown).
  - When valid, the echo shows `> Validate — ✓ valid YAML`.
  - On a problem, the cursor jumps there and the prompt line shows the error `Validate: line 3, col 7: <message>`.
  - Unsupported languages show the list of supported ones.
  - Positions are verified per parser: encoding/json byte offsets and XML/TOML byte columns are converted to characters, and goccy gives line:col directly. go-toml reports duplicate keys without a position, so the adapter points at the key's second assignment.
- **Validate on save** (`validate_on_save`, default on; ✓ option `view.toggleValidateOnSave` in the View menu):
  - Every Save and Save as goes through `saveValidated`. It validates as the language the file will have after saving: a hand-picked language wins, else the one detected from the target path.
  - A valid or unsupported file is written directly, and so is a file the validator can't check.
  - An invalid file is not written. A `choose` prompt asks `Fails validation (<problem>)  [v]alidate  [s]ave anyway  Esc`:
    - `v` shows the problem exactly like Ctrl+T, with the cursor moved onto it.
    - `s` writes the file.
    - Esc or any other key cancels.
  - It asks on every save; there's no exemption after "save anyway".
- **Choice prompt:** `Editor.choose(question, choice{key, label, run}...)` is the generic one-key question (modeChoice). The question is truncated so the answers stay visible.
- **Close file** (`file.close`, **Ctrl+W**) leaves an empty untitled buffer and asks first if there are unsaved changes.
- archtest: goccy/go-yaml and go-toml may only be imported by `adapters/validate`.

### Prompt line and status line

- The prompt line is always a gray bar (`StylePromptBar`, palette bright black with white text), whether idle, showing a message or focused.
- When there is no message it shows the idle text `write a command…`, with the prompt shortcut right-aligned (`StylePromptIdle`).
- An echo, info or error stays until the next key, click or paste in editor focus (`clearMessage` in `HandleKey`/`HandleMouse`/`HandlePaste`), then the line goes back to idle. Redraws and resizes don't clear it, and there are no timers for messages (the click ripple and the indent guide are the only animations, see below).
- The status line starts with `sie <version> - `, where the version is `buildinfo.Info.Display()`: the release tag (`v0.2.1`) for tagged builds, otherwise `dev-<commit7>[+dirty]`. The left part is truncated with `…` so it never overlaps the right part.

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

### Double and triple click

- `ports.MouseEvent.Clicks` is 1, 2 or 3 on a left press (0 otherwise). The tui `mouseTracker` counts presses on the same cell within 400 ms (using tcell's event time), and a fourth press starts over, so the core has no clock.
- In the text area a plain double click selects the run of same-class characters (`Document.WordRange`, the classes of word movement), and a triple click selects the line with its line break (`Document.LineRange`). Dragging afterwards extends by whole words or lines (`Editor.dragUnit`). Shift, Alt and column mode keep their own behavior.

### Click ripple

- `view.toggleClickRipple` ("Click ripple", ✓ option in the View menu, aliases `ripple`, `click effect`, `click highlight`, config `click_ripple`, default off). A left press in the text area (also Alt+click and column mode, never drags, menus or the prompt) starts a wave: three concentric elliptical rings (a column counts half a row, since cells are twice as tall as wide) spread to about 4 rows over 500 ms and fade. At most 3 waves live at once, and a wave finishes whatever happens to the text.
- **Core:** `Config.Now` is the clock (nil means `time.Now`), and tests inject a fake one. `ripple.go` turns age and distance into a level 1-3 per cell (`rippleLevel`), clipped to the text rows. `View` adds them as `ViewModel.Tints` and sets `ViewModel.Animate` while any wave is alive. `Poll` drops finished waves.
- **Adapter:** the painter draws the runs, then `tint` re-puts each tinted cell with a background from `rippleRamp` (RGB, so tcell picks the nearest color on small palettes), keeping its text, foreground and attributes. The loop selects on events and a frame timer (33 ms) that exists only while `Animate` is true, so an idle editor wakes for nothing.

### Indent guide

- `view.toggleIndentGuide` ("Indent guide", ✓ option in the View menu, aliases `alignment guide`, `indent flash`, config `indent_guide`, default off). When an edit moves the cursor line's *top* (the screen column of its first glyph that is not a space or tab, `editor.IndentCol`), a vertical guide is shown at that column over all text rows for 300 ms, to see how the line aligns with the others (Python, YAML).
- **Trigger** (`trackIndent`, run after every input event next to `syncHighlight`): the document version changed, the cursor line has a top before and after, and the column differs. Moving the cursor and editing blank lines never fire it, and Enter's auto-indent fires only if the indent really changed. Undo and redo count.
- **Drawing** (`drawGuide`, after the text and the ripple): a line whose top is at the guide column shows that glyph in reverse (`StyleGuideMatch`), other text at that column is only tinted (Tint level 4, gray background), and every other cell (short or blank lines, leading whitespace, rows past the end) shows `|` in `StyleGuide`. The guide uses `Animate` like the ripple and disappears in `Poll`.

### Multiple cursors

- **Alt+click** in the text area adds a cursor (`Document.AddCursor`); the new one becomes the primary cursor (terminal cursor, viewport follows), the old primary is kept as a secondary one. Alt+click on an existing cursor removes it.
- `Document` holds the primary `sel` plus `extra []Selection` (sorted, never overlapping; `mergeCursors` drops collisions, the primary wins). `SetSelection` removes the secondary cursors, so clicks, find, go to line, select all… go back to one cursor.
- `forEachCursor` runs a single-cursor operation once per cursor, from the last in the document to the first; `replace` shifts the other cursors (`mapReplace`). Multi-cursor: `Insert` (typing, Tab, paste), `InsertEach`, `InsertNewline`, `Backspace`, `DeleteForward`, `DeleteSelection`, `Move`, `MovePage`. Other operations act on the primary selection only.
- Undo: the history state is every cursor (`cursors`), so undo/redo restores them all. One run is one undo step (typing bursts still coalesce); `History.Amend` sets the final state after the run.
- App: Esc removes the secondary cursors first (a second Esc collapses the selection). `Dispatch` removes them before every non-silent command without `Command.MultiCursor` (set on Copy, Cut, Paste, Undo, Redo and the column-mode toggle).
- View: secondary selections are drawn as selections, each secondary cursor as one `StyleCursorSecondary` cell (also at a line end); the status line shows `(N cursors)`.
- Terminals: Alt+click needs Option-as-Meta on macOS; some Linux desktops grab Alt+click to move windows.

### Rectangular (column) selection

- A rectangle is **one `Selection` per line** on top of the multi-cursor code, so editing and undo need no new logic. `Document.SelectRect(aLine, aVCol, hLine, hVCol)` (`domain/editor/rect.go`) builds it from two corners given as line and screen column (`Layout`/`OffsetAtCol` rules: a glyph is inside when its first column is in `[left, right)`; a line shorter than the left edge gets an empty cursor at its end). The head line's selection is the primary one, and its cursor is on the dragged side. The block is read from the buffer once, because asking for each line's offset is quadratic.
- `rectState` keeps the corners in **virtual** columns (they may lie past a short line's end) together with the selections it produced. `ExtendRect(dLine, dVCol)` moves the head corner and reselects. The rectangle is *active* only while the document's selections are exactly the ones it produced (`activeRect`), so any other selection change or edit ends it with no explicit clearing.
- **Gestures** (app, `rect.go`): **Alt+drag** (the press is only recorded in `Editor.rectDrag`; a release without movement is the old Alt+click toggle), and **Column selection** (`selection.toggleColumnMode`, ✓ option, **Ctrl+U**, session only, status line shows `COLUMN`): a plain drag draws a rectangle and Shift+←/→/↑/↓ call `ExtendRect`. Alt+Shift+arrows stay word/jump selection.
- **Clipboard:** with several cursors, Copy/Cut put `SelectedTexts()` (document order, empty selections kept as empty lines) joined with `\n` on the clipboard, and say "nothing selected" if all are empty. Paste with N cursors splits the text into lines (a trailing line break first counts as an empty last line, then as a terminator); if there are N lines, `InsertEach` puts line *i* on the *i*-th cursor in document order (`forEachCursor` exposes the rank), else the whole text goes to every cursor. Copy, Cut and the toggle are `MultiCursor` commands.
- **Cost:** `forEachCursor` snaps the other cursors once at the end of a run (not after every edit). Selecting any height is instant; typing into a 5,000-line rectangle takes about 0.5 s (piece table, O(pieces) per edit), 20,000 lines about 8 s.

### Default keymap (v0)

| Key | Command | Key | Command |
|---|---|---|---|
| Ctrl+N | New | Ctrl+F | Find |
| Ctrl+O | Open | Ctrl+R | Replace |
| Ctrl+E | Open recent | Ctrl+W | Close file |
| Ctrl+T | Validate | Ctrl+U | Column selection (toggle) |
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
- Free Ctrl keys for future commands: **B, P** (E Open recent, T Validate, U Column selection, W Close file). Ctrl+B is the tmux prefix, so prefer P.
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

## CI and releases

- **`.github/workflows/ci.yml`** runs on pushes to `main`, on pull requests, and before every release:
  - vet and tests on Linux, macOS and Windows (`-race` except on Windows);
  - gofmt;
  - `go mod tidy` and `go mod vendor` must produce no diff;
  - `go mod verify`;
  - the six-target `CGO_ENABLED=0` build;
  - the no-cgo check.
- **`.github/workflows/release.yml`** runs on a pushed `v*` tag, or by hand (`workflow_dispatch`, with an existing tag, e.g. `gh workflow run release.yml -f tag=v0.5.1`). It runs CI, then GoReleaser **v2.18.2** with `.goreleaser.yaml`.
  - GoReleaser builds six static binaries with `-trimpath -mod=vendor`. buildinfo is set through ldflags: `version={{.Tag}}`, `commit`, `date`.
  - It produces `tar.gz` archives (`zip` on Windows) containing the binary, README and LICENSE, plus `checksums.txt`, a changelog from git, and the GitHub release.
  - The tag's sources are checked out in `src`, while the config and LICENSE come from the workflow's own ref in `cfg` (`--config $GITHUB_WORKSPACE/cfg/.goreleaser.yaml`, and an absolute `LICENSE_DIR`, since GoReleaser can't archive `../` paths). That way older tags, which have neither, still build with a clean tree.
- **Package managers**, published by GoReleaser on every release:
  - **Homebrew:** a cask in `avero-it/homebrew-tap` (`Casks/sie.rb`, macOS and Linux). A `postflight` hook removes the macOS quarantine flag, since the binaries are unsigned.
  - **Scoop:** a manifest in `avero-it/scoop-bucket` (`sie.json`).
  - **Linux packages:** `.deb`, `.rpm` and `.apk` (nfpm, `/usr/bin/sie` plus the LICENSE), attached to the release and covered by `checksums.txt`.
  - **The token:** writing to the tap and bucket needs the repository secret **`TAP_GITHUB_TOKEN`** in avero-it/sie: a fine-grained PAT, resource owner `avero-it`, access to only those two repos, Contents read and write. Without it, `skip_upload` turns those two steps off and the release still succeeds. When it expires, create a new one the same way and update the secret.
- **apt repository** (`.github/workflows/apt.yml`, `scripts/build-apt-repo.sh`): a signed repo at `https://avero-it.github.io/sie` (GitHub Pages, source "GitHub Actions"), suite `stable`, component `main`, amd64 and arm64.
  - It is a **separate workflow**, because releases are immutable and a failure there must never force a new version. It runs after a successful Release (`workflow_run`) and by hand: `gh workflow run apt.yml`.
  - Each run downloads the `.deb` of **every** release, rebuilds the whole repo (apt-ftparchive, signed with the secret **`APT_GPG_PRIVATE_KEY`**), installs `sie` from it as a smoke test, then deploys. A Pages deploy replaces the whole site, so nothing is incremental.
  - Without the secret the workflow only prints a notice. Key: RSA 4096, no passphrase, no expiry, uid `sie apt repository`. To rotate it, generate a new key, update the secret, rerun the workflow; users must fetch the new `sie.gpg` again.
  - `.rpm` and `.apk` are only release downloads (no repository).
- **First package-manager release:** v0.6.2. v0.6.0 predates the `TAP_GITHUB_TOKEN` secret, and v0.6.1 failed on the tap step (see below), so the tap and bucket start at v0.6.2. Releases are immutable: a published tag can't be re-run, a fix needs a new version. Never put two release tags on one commit: GoReleaser picks the highest and fails if that release already exists.
- **GoReleaser templates:** in `repository.token` and `skip_upload` use `.Env.NAME` (`envOrDefault` is not defined there). Archive and nfpm `src` paths do support `envOrDefault`.
- **To release:** `git tag -a vX.Y.Z -m … && git push origin vX.Y.Z`. To try the release locally: `goreleaser release --snapshot --clean` (output in `dist/`, which is git-ignored).
- **Pins:** actions are pinned to exact versions at least two weeks old (checkout v7.0.1, setup-go v7.0.0, goreleaser-action v7.2.3). Bump them deliberately.
- `.gitattributes` forces LF checkouts, so the Windows runners see the same bytes (gofmt, fuzz corpora).

## Conventions

- Go 1.26, standard `gofmt`/`goimports`. Packages are small and named for what they hold, not `utils`.
- Interfaces live in `ports` (consumer side), are small, and are only introduced when there is a second implementation or a test fake.
- Errors are wrapped with `%w` and carry context. No panics in `domain` or `app`. Invalid input returns errors.
- No global mutable state. Everything is constructed in `cmd/sie` and passed in.
- `context.Context` only at app/adapter boundaries (I/O), never in the domain.
- Tests are table-driven. Use `testing/fstest` or in-memory fakes for ports. Fuzz the piece table against a naive `[]byte` model. Use `tcell.SimulationScreen` golden tests for rendering (`-update` flag regenerates goldens).
- **Dependencies are vendored.** `vendor/` is committed, and the go command builds from it automatically, so builds need no network. After any change to `go.mod` (`go get`, version bumps), run `go mod tidy && go mod vendor` and commit `vendor/` in the same commit. `go mod verify` checks the module cache against `go.sum`. Never edit files under `vendor/`. archtest and gofmt checks skip it (`gofmt -l $(git ls-files '*.go' | grep -v '^vendor/')`).
- **New dependencies need justification** in the PR/commit message: pure Go, maintained, and not trivially replaceable by the stdlib. Current allow-list: `github.com/gdamore/tcell/v2` v2.13.10 (adapters/tui only; `cmd/sie` gets the screen through `tui.New`), `github.com/rivo/uniseg`, `golang.org/x/sys` (adapters only, for the Windows clipboard; already a tcell dependency), `github.com/spf13/viper` v1.21.0 (adapters/config only; requested for config files), `github.com/spf13/cobra` v1.10.2 (cmd/sie only; requested for the CLI), `github.com/alecthomas/chroma/v2` v2.27.0 (adapters/syntax only; requested for syntax highlighting; adds about 4 MB, since all lexers are embedded), `github.com/goccy/go-yaml` v1.19.2 (adapters/validate only; YAML errors with line and column), `github.com/pelletier/go-toml/v2` (adapters/validate; already a viper dependency). archtest enforces where tcell, viper, cobra and chroma may be imported. Pin exact versions that are at least two weeks old.

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
