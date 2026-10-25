// The demo scenarios. Every string shown on the fake screens (prompts, echoes,
// status line, error messages) is copied from what sie itself prints; check it
// against the app when a command changes. Screens follow sie-screen.js.
(function () {
  "use strict";

  // typeIn types into the prompt line, one character at a time.
  async function typeIn(d, label, text, ms) {
    for (var i = 0; i <= text.length; i++) {
      d.input(label, text.slice(0, i));
      if (i < text.length) await d.sleep(ms);
    }
  }

  var YAML6 = ["server:", "  host: localhost", "  port: 8080", "  tls:", "    enabled: true", "    cert: /etc/ssl/sie.pem"];

  var GO = [
    "package main",
    "",
    'import "fmt"',
    "",
    "// greet says hello.",
    "func greet(name string) {",
    '    fmt.Println("hi,", name, 42)',
    "}"
  ];

  var JSON4 = ["{", '  "name": "sie"', '  "port": 8080', "}"];
  var JSON_ERR = "Validate: line 3, col 3: invalid character '\"' after object key:value pair";
  var JSON_CHOICE = "Fails validation (li…  [v]alidate  [s]ave anyway  Esc";

  var defs = {};

  // ---- hero: a short tour -----------------------------------------------------

  defs.hero = {
    cols: 54,
    rows: 12,
    run: async function (d) {
      d.s.name = "demo.yaml"; d.s.lang = "YAML";
      d.say("Type: it is a plain, modeless editor.");
      await d.sleep(600);
      await d.type(YAML6.join("\n"), 26);
      await d.sleep(900);
      d.say("Click anywhere: a wave shows where (optional, click_ripple).");
      d.click(2, 9);
      await d.sleep(1100);
      d.say("Double-click selects a word.");
      d.select(1, 8, 17); d.cursor(1, 17); d.rippleAt(1, 12);
      await d.sleep(1500);
      d.say("Triple-click selects the line.");
      d.select(2, 0, -1); d.cursor(3, 0);
      await d.sleep(1500);
      d.say("Change a line's indent: the indent guide shows what it lines up with.");
      d.noSel(); d.cursor(4, 4);
      await d.sleep(700);
      d.s.lines[4] = "  enabled: true"; d.cursor(4, 2);
      d.s.guide = { col: 2, t0: performance.now() };
      await d.sleep(1900);
      d.say("Every command shows its shortcut: learn them as you go.");
      d.echo("Save file — wrote 6 lines", "Ctrl+S"); d.s.modified = false;
      await d.sleep(2600);
    },
    still: function (d) {
      d.s.name = "demo.yaml"; d.s.lang = "YAML"; d.setLines(YAML6);
      d.select(1, 8, 17); d.cursor(4, 4);
      d.echo("Save file — wrote 6 lines", "Ctrl+S");
      d.say("Double-click selects a word, triple-click a line; every command shows its shortcut.");
    }
  };

  // ---- modeless -----------------------------------------------------------------

  defs.modeless = {
    run: async function (d) {
      d.s.name = "notes.yaml"; d.s.lang = "YAML";
      d.say("No modes: just type.");
      await d.sleep(500);
      await d.type("title: sie\nauthor: you\ntags: 3", 45);
      await d.sleep(600);
      d.say("Move with the arrow keys.");
      d.press("↑"); d.cursor(1, 11); await d.sleep(450);
      d.press("↑"); d.cursor(0, 10); await d.sleep(450);
      d.say("Undo and redo.");
      d.press("Ctrl+Z"); d.setLines(["title: sie", "author: you", ""]); d.cursor(2, 0); await d.sleep(900);
      d.press("Ctrl+Y"); d.setLines(["title: sie", "author: you", "tags: 3"]); d.cursor(2, 7); await d.sleep(900);
      d.say("Save.");
      d.press("Ctrl+S"); d.echo("Save file — wrote 3 lines", "Ctrl+S"); d.s.modified = false;
      await d.sleep(2200);
    },
    still: function (d) {
      d.s.name = "notes.yaml"; d.s.lang = "YAML";
      d.setLines(["title: sie", "author: you", "tags: 3"]); d.cursor(2, 7);
      d.echo("Save file — wrote 3 lines", "Ctrl+S");
    }
  };

  // ---- command prompt ---------------------------------------------------------

  var ALPHA = [
    { t: "About sie" },
    { t: "Click ripple" },
    { t: "Close file", hint: "Ctrl+W" },
    { t: "Column selection", hint: "Ctrl+U" },
    { t: "Copy", hint: "Ctrl+C" },
    { t: "Cut", hint: "Ctrl+X" },
    { t: "Delete line", hint: "Ctrl+K" }
  ];

  defs.prompt = {
    run: async function (d) {
      d.s.name = "notes.txt"; d.s.invisibles = true;
      d.setLines(["x = 1   ", "y = 2  ", "z = 3"]); d.cursor(2, 5);
      d.say("Every command is one fuzzy search away.");
      await d.sleep(700);
      d.press("Ctrl+Alt+P"); d.input("> ", ""); d.popup(ALPHA, 0);
      await d.sleep(1300);
      d.popup(null);
      for (var i = 1; i <= 4; i++) { d.input("> ", "trim".slice(0, i)); await d.sleep(170); }
      d.popup([{ t: "Trim trailing spaces" }], 0);
      await d.sleep(900);
      d.press("Enter"); d.popup(null);
      d.setLines(["x = 1", "y = 2", "z = 3"]);
      d.echo("Trim trailing spaces — 2 lines trimmed");
      d.say("The echo says what it did (and teaches the shortcut).");
      await d.sleep(1600);
      d.say("Arguments can be typed inline.");
      d.press("Ctrl+Alt+P");
      for (var j = 1; j <= 6; j++) { d.input("> ", "goto 2".slice(0, j)); await d.sleep(150); }
      d.popup([{ t: "Go to line 2", hint: "Ctrl+G" }], 0);
      await d.sleep(800);
      d.press("Enter"); d.popup(null); d.cursor(1, 0);
      d.echo("Go to line 2", "Ctrl+G");
      await d.sleep(2000);
    },
    still: function (d) {
      d.s.name = "notes.txt"; d.s.invisibles = true;
      d.setLines(["x = 1   ", "y = 2  ", "z = 3"]); d.cursor(2, 5);
      d.input("> ", "trim"); d.popup([{ t: "Trim trailing spaces" }], 0);
    }
  };

  // ---- open recent --------------------------------------------------------------

  var RECENT = [
    { t: "cmd/sie/main.go" }, { t: "README.md" }, { t: "notes/todo.txt" }, { t: "internal/app/view.go" }
  ];

  defs.recent = {
    run: async function (d) {
      d.say("Ctrl+E lists the files you opened or saved, newest first.");
      await d.sleep(700);
      d.press("Ctrl+E"); d.input("Open recent: ", ""); d.popup(RECENT, 0);
      await d.sleep(1400);
      d.say("Type a few letters of the path: fuzzy matching.");
      var steps = [
        ["t", [RECENT[2], RECENT[3]]],
        ["to", [RECENT[2], RECENT[3]]],
        ["tod", [RECENT[2]]],
        ["todo", [RECENT[2]]]
      ];
      for (var i = 0; i < steps.length; i++) {
        d.input("Open recent: ", steps[i][0]); d.popup(steps[i][1], 0);
        await d.sleep(450);
      }
      await d.sleep(700);
      d.press("Enter"); d.popup(null);
      d.s.name = "todo.txt"; d.setLines(["buy milk", "call mom", "ship v0.9.0"]); d.cursor(0, 0);
      d.echo("Open recent notes/todo.txt — 3 lines", "Ctrl+E");
      await d.sleep(2200);
    },
    still: function (d) {
      d.input("Open recent: ", "todo"); d.popup([RECENT[2]], 0);
    }
  };

  // ---- large files --------------------------------------------------------------

  var BIG_HEAD = [
    "00:00:00 info  start",
    "00:00:01 info  listen :8080",
    "00:00:04 warn  slow query"
  ];
  var BIG_TAIL = [
    "23:59:57 info  request ok",
    "23:59:58 error timeout",
    "23:59:59 info  shutdown"
  ];
  var BIG_LINES = 13041772;

  defs.largefile = {
    cols: 60,
    rows: 10,
    run: async function (d) {
      d.s.name = "events.txt"; d.s.gutter = 9;
      d.setLines(BIG_HEAD); d.cursor(0, 0);
      d.say("A 1 GB file, 13 million lines: it opens at once and stays on disk.");
      await d.sleep(1800);
      d.press("Ctrl+End");
      d.setLines(BIG_TAIL); d.cursor(2, 22);
      d.s.base = BIG_LINES - 3;
      d.say("Jumping to the end is instant: only the lines you see are read.");
      await d.sleep(1700);
      d.say("Edit as usual.");
      await d.type("\n", 100);
      await d.type("23:59:59 info  bye", 40);
      d.s.modified = true;
      await d.sleep(1200);
      d.say("Save streams the file to disk and swaps it in atomically.");
      d.press("Ctrl+S"); d.s.modified = false;
      d.echo("Save file — wrote " + (BIG_LINES + 1) + " lines", "Ctrl+S");
      await d.sleep(3200);
    },
    still: function (d) {
      d.s.name = "events.txt"; d.s.gutter = 9; d.s.base = BIG_LINES - 3;
      d.setLines(BIG_TAIL); d.cursor(2, 22);
      d.echo("Save file — wrote " + BIG_LINES + " lines", "Ctrl+S");
    }
  };

  // ---- multiple cursors ---------------------------------------------------------

  var BULLETS = ["- alpha", "- beta", "- gamma"];

  defs.multicursor = {
    run: async function (d) {
      d.s.name = "list.md";
      d.setLines(BULLETS); d.cursor(0, 7);
      d.say("Alt+click adds a cursor.");
      await d.sleep(800);
      d.press("Alt+click"); d.s.cursors.unshift({ l: 1, c: 6 }); d.rippleAt(1, 6);
      await d.sleep(700);
      d.press("Alt+click"); d.s.cursors.unshift({ l: 2, c: 7 }); d.rippleAt(2, 7);
      await d.sleep(900);
      d.say("Everything you type goes to every cursor.");
      await d.type(";", 300);
      await d.sleep(900);
      d.press("Esc"); d.cursor(2, 8);
      d.say("Alt+drag selects a rectangle: here the bullets.");
      await d.sleep(700);
      d.press("Alt+drag");
      d.s.sels = [{ l: 0, a: 0, b: 2 }, { l: 1, a: 0, b: 2 }, { l: 2, a: 0, b: 2 }];
      await d.sleep(1300);
      d.press("Backspace");
      d.setLines(["alpha;", "beta;", "gamma;"]); d.noSel(); d.cursor(2, 0);
      await d.sleep(1200);
      d.say("Undo restores all of them at once.");
      d.press("Ctrl+Z"); d.cursor(2, 2); d.setLines(["- alpha;", "- beta;", "- gamma;"]);
      d.s.sels = [{ l: 0, a: 0, b: 2 }, { l: 1, a: 0, b: 2 }, { l: 2, a: 0, b: 2 }];
      await d.sleep(2000);
    },
    still: function (d) {
      d.s.name = "list.md"; d.setLines(["- alpha;", "- beta;", "- gamma;"]);
      d.s.cursors = [{ l: 2, c: 8 }, { l: 1, c: 7 }, { l: 0, c: 8 }];
    }
  };

  // ---- mouse ----------------------------------------------------------------------

  var PROSE = ["the quick brown fox", "jumps over the lazy dog", "and runs away"];

  defs.mouse = {
    run: async function (d) {
      d.s.name = "story.txt"; d.setLines(PROSE); d.cursor(0, 0);
      d.say("Double-click selects a word.");
      await d.sleep(700);
      d.s.pointer = { l: 0, c: 6 }; await d.sleep(350);
      d.select(0, 4, 9); d.cursor(0, 9);
      await d.sleep(1300);
      d.say("Triple-click selects the line.");
      d.s.pointer = { l: 1, c: 8 }; await d.sleep(350);
      d.select(1, 0, -1); d.cursor(2, 0);
      await d.sleep(1400);
      d.say("Drag after a double-click: it grows by whole words.");
      d.noSel(); d.cursor(0, 0);
      d.s.pointer = { l: 0, c: 6 }; await d.sleep(350);
      d.select(0, 4, 9); d.cursor(0, 9);
      await d.sleep(600);
      d.s.pointer = { l: 0, c: 12 }; d.select(0, 4, 19); d.cursor(0, 19);
      await d.sleep(650);
      d.s.pointer = { l: 1, c: 7 };
      d.s.sels = [{ l: 0, a: 4, b: -1 }, { l: 1, a: 0, b: 10 }]; d.cursor(1, 10);
      await d.sleep(1800);
      d.s.pointer = null;
    },
    still: function (d) {
      d.s.name = "story.txt"; d.setLines(PROSE); d.cursor(1, 10);
      d.s.sels = [{ l: 0, a: 4, b: -1 }, { l: 1, a: 0, b: 10 }];
    }
  };

  // ---- replace ------------------------------------------------------------------

  defs.replace = {
    run: async function (d) {
      d.s.name = "todo.txt"; d.setLines(["foo bar", "foo baz", "foo qux"]); d.cursor(0, 0);
      d.say("Ctrl+R asks what to replace, and with what.");
      await d.sleep(700);
      d.press("Ctrl+R");
      await typeIn(d, "Replace: ", "foo", 220);
      await d.sleep(300);
      d.press("Enter");
      await typeIn(d, "With: ", "bar", 220);
      await d.sleep(300);
      d.press("Enter");
      d.say("Then it asks about each match: y, n, a (all) or q (quit).");
      var q = 'Replace "foo" with "bar"? (y/n/a/q)';
      d.choice(q); d.select(0, 0, 3); d.cursor(0, 3);
      await d.sleep(1300);
      d.press("y"); d.setLines(["bar bar", "foo baz", "foo qux"]); d.select(1, 0, 3); d.cursor(1, 3);
      await d.sleep(1100);
      d.press("n"); d.select(2, 0, 3); d.cursor(2, 3);
      await d.sleep(1100);
      d.press("y"); d.setLines(["bar bar", "foo baz", "bar qux"]); d.noSel(); d.cursor(2, 3);
      d.echo("Replace foo bar — 2 matches replaced", "Ctrl+R");
      await d.sleep(1600);
      d.say("The whole session is one undo step.");
      d.press("Ctrl+Z"); d.setLines(["foo bar", "foo baz", "foo qux"]); d.cursor(0, 3);
      d.echo("Undo", "Ctrl+Z");
      await d.sleep(2000);
    },
    still: function (d) {
      d.s.name = "todo.txt"; d.setLines(["bar bar", "foo baz", "foo qux"]);
      d.select(1, 0, 3); d.cursor(1, 3);
      d.choice('Replace "foo" with "bar"? (y/n/a/q)');
    }
  };

  // ---- live search ---------------------------------------------------------------

  var PROSE = [
    "The quick brown fox",
    "jumps over the lazy dog.",
    "The dog barks; the fox runs.",
    "Another fox appears here.",
    "The end."
  ];

  defs.search = {
    run: async function (d) {
      d.s.name = "story.txt"; d.setLines(PROSE); d.cursor(0, 0);
      d.say("Ctrl+F searches as you type: the view jumps to the nearest match.");
      await d.sleep(800);
      d.press("Ctrl+F"); d.input("Find: ", "");
      await d.sleep(500);
      var q = "";
      // each step: the text typed, the match it selects, and where the cursor ends
      var steps = [["f", 0, 16, 17], ["fo", 0, 16, 18], ["fox", 0, 16, 19]];
      for (var i = 0; i < steps.length; i++) {
        q = steps[i][0]; d.find(q); d.input("Find: ", q);
        d.select(steps[i][1], steps[i][2], steps[i][3]); d.cursor(steps[i][1], steps[i][3]);
        await d.sleep(600);
      }
      d.say("Every other match stays underlined.");
      await d.sleep(1200);
      d.say("Keep typing: it moves on to the match that fits.");
      q = "fox r"; d.find(q); d.input("Find: ", q);
      d.select(2, 19, 24); d.cursor(2, 24);
      await d.sleep(1500);
      d.say("Backspace searches again from where you started.");
      d.press("Backspace"); q = "fox"; d.find(q); d.input("Find: ", q);
      d.select(0, 16, 19); d.cursor(0, 19);
      await d.sleep(1300);
      d.say("No match: the cursor stays where it was.");
      q = "foxx"; d.find(q); d.input("Find: ", q, "not found");
      d.noSel(); d.cursor(0, 0);
      await d.sleep(1700);
      d.say("Esc puts everything back. Enter keeps the match.");
      d.press("Esc"); d.find(null); d.idle();
      await d.sleep(1500);
      d.press("Ctrl+F"); d.input("Find: ", "the");
      d.find("the"); d.select(0, 0, 3); d.cursor(0, 3);
      await d.sleep(800);
      d.press("Enter"); d.find(null); d.echo("Find the", "Ctrl+F");
      await d.sleep(2200);
    },
    still: function (d) {
      d.s.name = "story.txt"; d.setLines(PROSE);
      d.find("fox"); d.input("Find: ", "fox");
      d.select(0, 16, 19); d.cursor(0, 19);
    }
  };

  // ---- validation ---------------------------------------------------------------

  defs.validate = {
    cols: 54,
    run: async function (d) {
      d.s.name = "config.json"; d.s.lang = "JSON"; d.setLines(JSON4); d.cursor(0, 0);
      d.say("Ctrl+T checks the file as JSON, YAML, TOML or XML.");
      await d.sleep(900);
      d.press("Ctrl+T"); d.cursor(2, 2); d.error(JSON_ERR);
      d.say("It jumps to the first problem.");
      await d.sleep(2200);
      d.say("Fix it...");
      d.cursor(1, 15); await d.sleep(500);
      await d.type(",", 300); d.s.modified = true; d.idle();
      await d.sleep(500);
      d.press("Ctrl+T"); d.echo("Validate — ✓ valid JSON", "Ctrl+T");
      await d.sleep(1600);
      d.say("Saving a broken file asks first.");
      d.s.lines[1] = '  "name": "sie"'; d.cursor(1, 15);
      d.press("Backspace"); await d.sleep(700);
      d.press("Ctrl+S"); d.choice(JSON_CHOICE);
      await d.sleep(2600);
    },
    still: function (d) {
      d.s.name = "config.json"; d.s.lang = "JSON"; d.setLines(JSON4); d.cursor(2, 2);
      d.error(JSON_ERR);
    }
  };

  // ---- syntax highlighting ----------------------------------------------------

  defs.syntax = {
    run: async function (d) {
      d.s.name = "main.go"; d.s.lang = "Go"; d.s.colored = false;
      d.say("Colors for every language chroma knows, found by file name.");
      await d.sleep(600);
      await d.type(GO.join("\n"), 20);
      await d.sleep(350);
      d.s.colored = true;
      d.say("Highlighting runs in the background: typing never waits for it.");
      await d.sleep(1800);
      d.say("Switch it off, or pick a language yourself.");
      d.press("Ctrl+Alt+P");
      for (var i = 1; i <= 6; i++) { d.input("> ", "syntax".slice(0, i)); await d.sleep(130); }
      await d.sleep(300);
      d.press("Enter"); d.s.colored = false; d.echo("  Syntax highlighting — off");
      await d.sleep(1300);
      d.press("Ctrl+Alt+P");
      for (var j = 1; j <= 6; j++) { d.input("> ", "syntax".slice(0, j)); await d.sleep(110); }
      await d.sleep(250);
      d.press("Enter"); d.s.colored = true; d.echo("✓ Syntax highlighting — on");
      await d.sleep(2200);
    },
    still: function (d) {
      d.s.name = "main.go"; d.s.lang = "Go"; d.setLines(GO); d.cursor(7, 1);
    }
  };

  // ---- careful with files ---------------------------------------------------------

  defs.files = {
    run: async function (d) {
      d.say("Open a Windows file with a BOM...");
      await d.sleep(800);
      d.s.name = "notes.txt"; d.s.bom = true; d.s.eol = "CRLF";
      d.setLines(["hello  world", "\tsecond^A�", ""]); d.cursor(0, 0);
      d.echo("Open file notes.txt — 3 lines", "Ctrl+O");
      await d.sleep(1700);
      d.say("Control characters and invalid bytes are always visible.");
      await d.sleep(1600);
      d.say("Show spaces and tabs when you need to.");
      d.press("Ctrl+Alt+P");
      d.s.invisibles = true; d.echo("✓ Invisible characters — shown");
      await d.sleep(2000);
      d.say("Edit and save: the BOM and the CRLF endings are kept.");
      d.cursor(0, 12);
      await d.type("!", 400); d.s.modified = true;
      await d.sleep(600);
      d.press("Ctrl+S"); d.s.modified = false; d.echo("Save file — wrote 3 lines", "Ctrl+S");
      await d.sleep(2400);
    },
    still: function (d) {
      d.s.name = "notes.txt"; d.s.bom = true; d.s.eol = "CRLF"; d.s.invisibles = true;
      d.setLines(["hello  world", "\tsecond^A�", ""]);
      d.echo("✓ Invisible characters — shown");
    }
  };

  // ---- visual helpers -----------------------------------------------------------

  defs.visual = {
    run: async function (d) {
      d.s.name = "demo.yaml"; d.s.lang = "YAML"; d.setLines(YAML6); d.cursor(0, 0);
      d.say("Click ripple (optional): every click sends out a wave.");
      await d.sleep(700);
      d.click(2, 9); await d.sleep(900);
      d.click(4, 8); await d.sleep(1300);
      d.say("Indent guide (optional): change a line's indent...");
      d.noSel(); d.cursor(4, 4);
      await d.sleep(600);
      d.press("Shift+Tab");
      d.s.lines[4] = "  enabled: true"; d.cursor(4, 2); d.s.modified = true;
      d.s.guide = { col: 2, t0: performance.now() };
      d.say("...and a guide flashes at its new column, showing what it lines up with.");
      await d.sleep(2600);
    },
    still: function (d) {
      d.s.name = "demo.yaml"; d.s.lang = "YAML"; d.setLines(YAML6); d.cursor(4, 2);
      d.s.lines[4] = "  enabled: true";
    }
  };

  // ---- column ruler and wrap ---------------------------------------------------

  var LONG = [
    "The ruler follows the scroll: columns stay right even when a long line is cut off.",
    "A short line.",
    "Wrapped lines break at spaces, and the rows after the first one are marked in the gutter."
  ];

  defs.wrap = {
    run: async function (d) {
      d.s.name = "notes.txt"; d.setLines(LONG); d.cursor(0, 0);
      d.say("The column ruler on the top row shows where you are.");
      await d.sleep(1200);
      d.say("Scroll sideways and its numbers move with the text.");
      d.press("End");
      var end = LONG[0].length, width = d.cols - (d.s.gutter || 3);
      var target = end - width + 1;
      d.cursor(0, end);
      for (var x = 0; x <= target; x += 3) { d.s.left = Math.min(x, target); await d.sleep(60); }
      d.s.left = target;
      await d.sleep(1600);
      d.say("Wrap lines (optional): long lines continue on the next row, marked with ↳.");
      d.press("Ctrl+P Z");
      d.s.wrap = true; d.s.left = 0; d.cursor(0, 0);
      d.echo("✓ Wrap lines — on", "Ctrl+P Z");
      await d.sleep(2800);
      d.say("The ruler gives its row back while wrapping. Ctrl+P Z again turns it off.");
      d.press("Ctrl+P Z");
      d.s.wrap = false;
      d.echo("  Wrap lines — off", "Ctrl+P Z");
      await d.sleep(2200);
    },
    still: function (d) {
      d.s.name = "notes.txt"; d.setLines(LONG); d.cursor(0, 0);
      d.s.wrap = true;
      d.echo("✓ Wrap lines — on", "Ctrl+P Z");
    }
  };

  // ---- split view ----------------------------------------------------------------

  var GOSPLIT = [
    "package main", "", "// Config holds the server settings.", "type Config struct {",
    "    Host string", "    Port int", "}", "", "func load() Config {",
    "    return Config{\"localhost\", 8080}", "}", "", "func main() {",
    "    cfg := load()", "    fmt.Println(cfg.Host)", "    fmt.Println(cfg.Port)", "}"
  ];

  // swapPane moves the cursor to the other pane: each pane keeps its own scroll.
  function swapPane(d) {
    var s = d.s, t = s.top;
    s.top = s.top2; s.top2 = t; s.focus2 = !s.focus2;
  }

  defs.split = {
    cols: 54,
    rows: 14,
    run: async function (d) {
      d.s.name = "main.go"; d.s.lang = "Go"; d.setLines(GOSPLIT);
      d.s.top = 3; d.cursor(5, 12);
      d.say("One file, one cursor. Sometimes you want to see two places at once.");
      await d.sleep(1600);
      d.say("Split view shows the same document in two stacked panes.");
      d.press("Ctrl+P X");
      d.s.split = true; d.s.top2 = 3;
      d.echo("✓ Split view — on", "Ctrl+P X");
      await d.sleep(1800);
      d.say("Each pane scrolls on its own: the lower one goes to main(), the upper keeps the struct.");
      for (var t = 3; t <= 12; t += 3) { d.s.top2 = t; await d.sleep(260); }
      await d.sleep(1500);
      d.say("Click in the lower pane and the cursor moves there. Edits go to the one document.");
      swapPane(d); d.click(15, 25);
      d.idle();
      await d.sleep(900);
      await d.type(" // TCP", 70);
      await d.sleep(1200);
      d.say("Ctrl+P 2 switches pane without the mouse.");
      d.press("Ctrl+P 2");
      swapPane(d); d.cursor(5, 12);
      d.echo("Switch pane", "Ctrl+P 2");
      await d.sleep(2000);
      d.say("Ctrl+P X again goes back to one pane.");
      d.press("Ctrl+P X");
      d.s.split = false;
      d.echo("  Split view — off", "Ctrl+P X");
      await d.sleep(2200);
    },
    still: function (d) {
      d.s.name = "main.go"; d.s.lang = "Go"; d.setLines(GOSPLIT);
      d.s.split = true; d.s.top = 3; d.s.top2 = 12; d.cursor(5, 12);
      d.echo("✓ Split view — on", "Ctrl+P X");
    }
  };

  // ---- one binary ----------------------------------------------------------------

  defs.binary = {
    cols: 56,
    rows: 10,
    shell: true,
    run: async function (d) {
      d.shell();
      d.say("One file. Build it once for any platform, copy it, run it.");
      await d.sleep(600);
      await d.typeShell("ls -lh sie", 55);
      d.sh("-rwxr-xr-x 1 you you 11M Oct 18 18:50 sie");
      await d.sleep(900);
      await d.typeShell("file sie", 55);
      d.sh("sie: ELF 64-bit LSB executable, x86-64, version 1");
      d.sh("(SYSV), statically linked, Go BuildID=..., stripped");
      await d.sleep(1100);
      await d.typeShell("ldd sie", 55);
      d.sh("        not a dynamic executable");
      await d.sleep(1100);
      await d.typeShell("scp sie server:/usr/local/bin/", 40);
      await d.sleep(700);
      await d.typeShell("ssh server sie version | head -1", 40);
      d.sh("sie v0.9.0");
      await d.sleep(2600);
    },
    still: function (d) {
      d.shell();
      d.sh("$ ls -lh sie");
      d.sh("-rwxr-xr-x 1 you you 11M Oct 18 18:50 sie");
      d.sh("$ ldd sie");
      d.sh("        not a dynamic executable");
      d.sh("$ ssh server sie version | head -1");
      d.sh("sie v0.9.0");
    }
  };

  window.SieScreens.init(defs);
})();
