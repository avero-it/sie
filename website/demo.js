// A scripted replay of sie in a fake terminal. It only draws text cells: the
// ripple and indent-guide math follows internal/core/app/{ripple,guide}.go.
(function () {
  "use strict";

  var host = document.getElementById("demo");
  var caption = document.getElementById("demo-caption");
  if (!host) return;

  var W = 54; // columns
  var H = 12; // rows: menu bar, 9 text rows, prompt line, status line
  var TEXT_TOP = 1;
  var TEXT_ROWS = 9;
  var GUTTER = 3;
  var MENU = " File  Edit  Selection  Search  View  Help";

  var RIPPLE_MS = 500;
  var GUIDE_MS = 700; // the editor uses 300 ms; a little longer here so it can be seen

  var state;
  var reduced = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  function reset() {
    state = {
      lines: [""],
      cur: { l: 0, c: 0 },
      sel: null, // {l, a, b}: a line with columns a..b selected, b = -1 for the line break too
      selLine: false,
      ripple: null, // {x, y, t0}
      guide: null, // {col, t0}
      prompt: null, // {text, hint}
      status: "demo.yaml",
      modified: false
    };
  }

  function sleep(ms) {
    return new Promise(function (r) { setTimeout(r, ms); });
  }

  function say(text) {
    if (caption) caption.textContent = text;
  }

  // ---- drawing -----------------------------------------------------------

  function pad(s, n) {
    while (s.length < n) s += " ";
    return s.slice(0, n);
  }

  function esc(ch) {
    return ch === "<" ? "&lt;" : ch === ">" ? "&gt;" : ch === "&" ? "&amp;" : ch;
  }

  function rippleLevel(dx, dy, age) {
    var p = age / RIPPLE_MS;
    if (p < 0 || p >= 1) return 0;
    var d = Math.hypot(dx / 2, dy);
    var lead = 4 * (1 - (1 - p) * (1 - p));
    var best = 0;
    for (var k = 0; k < 3; k++) {
      var off = Math.abs(d - (lead - k));
      if (off >= 0.7) continue;
      best = Math.max(best, (1 - p) * (1 - 0.3 * k) * (1 - off / 0.7));
    }
    return best > 0.5 ? 3 : best > 0.25 ? 2 : best > 0.08 ? 1 : 0;
  }

  function indentCol(s) {
    var m = /^ */.exec(s)[0].length;
    return m === s.length ? -1 : m;
  }

  // Syntax class per character of a YAML-ish line.
  function syntax(line) {
    var cls = new Array(line.length);
    var colon = line.indexOf(":");
    var start = indentCol(line);
    for (var i = 0; i < line.length; i++) {
      if (colon >= 0 && i >= start && i < colon) cls[i] = "k";
      else if (colon >= 0 && i > colon) {
        var v = line.slice(colon + 1).trim();
        cls[i] = /^(\d+|true|false)$/.test(v) ? "n" : "v";
      } else cls[i] = "";
    }
    return cls;
  }

  function render(now) {
    var cells = [];
    var r, c;
    for (r = 0; r < H; r++) {
      cells.push([]);
      for (c = 0; c < W; c++) cells[r].push([" ", ""]);
    }
    function put(r, c, ch, cls) {
      if (r >= 0 && r < H && c >= 0 && c < W) cells[r][c] = [ch, cls];
    }
    function puts(r, c, s, cls) {
      for (var i = 0; i < s.length; i++) put(r, c + i, s[i], cls);
    }

    // menu bar
    for (c = 0; c < W; c++) put(0, c, " ", "m");
    puts(0, 0, MENU, "m");

    // text rows
    var guide = state.guide && now - state.guide.t0 < GUIDE_MS ? state.guide : null;
    for (r = 0; r < TEXT_ROWS; r++) {
      var y = TEXT_TOP + r;
      var gl = String(r + 1).padStart(2, " ") + " ";
      if (r < state.lines.length) puts(y, 0, gl, r === state.cur.l ? "gc" : "g");
      var line = r < state.lines.length ? state.lines[r] : null;
      var cls = line === null ? [] : syntax(line);
      for (c = 0; c < W - GUTTER; c++) {
        var x = GUTTER + c;
        if (line !== null && c < line.length) put(y, x, line[c], cls[c]);
        else put(y, x, "·", "e");
      }
      if (line !== null && state.sel && state.sel.l === r) {
        var b = state.sel.b < 0 ? line.length + 1 : state.sel.b;
        for (c = state.sel.a; c < b; c++) {
          put(y, GUTTER + c, c < line.length ? line[c] : " ", "sel");
        }
      }
    }

    // ripple, then the indent guide (both over the text)
    if (state.ripple) {
      var age = now - state.ripple.t0;
      if (age >= RIPPLE_MS) state.ripple = null;
      else {
        for (r = TEXT_TOP; r < TEXT_TOP + TEXT_ROWS; r++) {
          for (c = 0; c < W; c++) {
            var lv = rippleLevel(c - state.ripple.x, r - state.ripple.y, age);
            if (lv) cells[r][c][1] = (cells[r][c][1] + " t" + lv).trim();
          }
        }
      }
    }
    if (guide) {
      var gx = GUTTER + guide.col;
      for (r = 0; r < TEXT_ROWS; r++) {
        var ln = r < state.lines.length ? state.lines[r] : null;
        var ch = ln !== null && guide.col < ln.length ? ln[guide.col] : " ";
        if (ln !== null && indentCol(ln) === guide.col) put(TEXT_TOP + r, gx, ch, "gm");
        else if (ln !== null && ch !== " ") cells[TEXT_TOP + r][gx][1] += " t4";
        else put(TEXT_TOP + r, gx, "|", "gd");
      }
    }

    // cursor
    if (!state.sel && state.cur) {
      var cl = state.lines[state.cur.l] || "";
      var cc = state.cur.c;
      put(TEXT_TOP + state.cur.l, GUTTER + cc, cc < cl.length ? cl[cc] : " ", "cur");
    }

    // prompt line and status line
    for (c = 0; c < W; c++) put(TEXT_TOP + TEXT_ROWS, c, " ", state.prompt ? "p" : "pi");
    if (state.prompt) {
      puts(TEXT_TOP + TEXT_ROWS, 0, "> " + state.prompt.text, "p");
      puts(TEXT_TOP + TEXT_ROWS, W - state.prompt.hint.length - 1, state.prompt.hint, "p");
    } else {
      puts(TEXT_TOP + TEXT_ROWS, 0, "write a command…", "pi");
      puts(TEXT_TOP + TEXT_ROWS, W - 14, "Ctrl+Alt+P", "pi");
    }
    for (c = 0; c < W; c++) put(H - 1, c, " ", "s");
    var st = "sie v1 - " + state.status + (state.modified ? " ●" : "");
    puts(H - 1, 0, st, "s");
    var pos = "Ln " + (state.cur.l + 1) + ", Col " + (state.cur.c + 1) + "   YAML   LF";
    puts(H - 1, W - pos.length - 1, pos, "s");

    // to HTML, merging runs of the same class
    var out = [];
    for (r = 0; r < H; r++) {
      var html = "";
      var runCls = null;
      for (c = 0; c < W; c++) {
        var cl2 = cells[r][c][1];
        if (cl2 !== runCls) {
          if (runCls !== null) html += "</span>";
          html += '<span class="' + cl2 + '">';
          runCls = cl2;
        }
        html += esc(cells[r][c][0]);
      }
      out.push(html + "</span>");
    }
    host.innerHTML = '<pre aria-hidden="true">' + out.join("\n") + "</pre>";
  }

  // ---- script ------------------------------------------------------------

  async function type(text, ms) {
    for (var i = 0; i < text.length; i++) {
      var ch = text[i];
      if (ch === "\n") {
        state.lines.push("");
        state.cur = { l: state.cur.l + 1, c: 0 };
      } else {
        var ln = state.lines[state.cur.l];
        state.lines[state.cur.l] = ln.slice(0, state.cur.c) + ch + ln.slice(state.cur.c);
        state.cur.c++;
      }
      state.modified = true;
      await sleep(ms);
    }
  }

  function click(l, c) {
    state.sel = null;
    state.cur = { l: l, c: c };
    state.ripple = { x: GUTTER + c, y: TEXT_TOP + l, t0: performance.now() };
  }

  async function script() {
    for (;;) {
      reset();
      say("Type: it is a plain, modeless editor.");
      await sleep(600);
      await type("server:\n  host: localhost\n  port: 8080\n  tls:\n    enabled: true\n    cert: /etc/ssl/sie.pem", 28);
      await sleep(900);

      say("Click anywhere: a wave shows where (optional, click_ripple).");
      click(2, 9);
      await sleep(1100);

      say("Double-click selects a word.");
      state.sel = { l: 1, a: 8, b: 17 };
      state.ripple = { x: GUTTER + 12, y: TEXT_TOP + 1, t0: performance.now() };
      state.cur = { l: 1, c: 17 };
      await sleep(1500);

      say("Triple-click selects the line.");
      state.sel = { l: 2, a: 0, b: -1 };
      state.cur = { l: 3, c: 0 };
      await sleep(1500);

      say("Change a line's indent: the indent guide shows what it lines up with.");
      state.sel = null;
      state.cur = { l: 4, c: 4 };
      await sleep(700);
      state.lines[4] = "  enabled: true";
      state.cur = { l: 4, c: 2 };
      state.guide = { col: 2, t0: performance.now() };
      await sleep(1900);

      say("Every command shows its shortcut: learn them as you go.");
      state.prompt = { text: "Save file — wrote 6 lines", hint: "Ctrl+S" };
      state.modified = false;
      await sleep(2600);
      state.prompt = null;
      await sleep(400);
    }
  }

  // ---- start -------------------------------------------------------------

  reset();
  if (reduced) {
    // One still frame: no motion for people who asked for none.
    state.lines = ["server:", "  host: localhost", "  port: 8080", "  tls:", "    enabled: true", "    cert: /etc/ssl/sie.pem"];
    state.cur = { l: 4, c: 4 };
    state.sel = { l: 1, a: 8, b: 17 };
    state.prompt = { text: "Save file — wrote 6 lines", hint: "Ctrl+S" };
    render(0);
    say("Double-click selects a word, triple-click a line; every command shows its shortcut.");
    return;
  }

  function frame() {
    render(performance.now());
    requestAnimationFrame(frame); // paused by the browser while the tab is hidden
  }
  frame();
  script();
})();
