// A tiny fake sie screen for the website demos. It draws a text grid from a
// state object; scenarios (demos.js) change the state over time. The ripple and
// indent-guide math follows internal/core/app/{ripple,guide}.go, and the
// screen layout follows internal/core/app/view.go.
(function () {
  "use strict";

  var CANCEL = { cancelled: true };
  var GUTTER = 3;
  var RIPPLE_MS = 500;
  var GUIDE_MS = 700; // the editor uses 300 ms; a little longer so it can be seen
  var KEYS = {
    go: "package import func var const type return if else for range struct switch case defer go".split(" "),
    json: ["true", "false", "null"]
  };

  function esc(ch) {
    return ch === "<" ? "&lt;" : ch === ">" ? "&gt;" : ch === "&" ? "&amp;" : ch;
  }

  function indentCol(s) {
    var m = /^ */.exec(s)[0].length;
    return m === s.length ? -1 : m;
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

  // Syntax classes per character: "k" keys/keywords, "v" strings, "n" numbers,
  // "c" comments, "t" types/builtins.
  function classify(lang, line) {
    var out = new Array(line.length);
    var i;
    for (i = 0; i < line.length; i++) out[i] = "";
    if (lang === "YAML") {
      var colon = line.indexOf(":");
      var start = indentCol(line);
      if (colon >= 0) {
        for (i = Math.max(start, 0); i < colon; i++) out[i] = "k";
        var val = line.slice(colon + 1).trim();
        for (i = colon + 1; i < line.length; i++) out[i] = /^(\d+|true|false)$/.test(val) ? "n" : "v";
      }
    } else if (lang === "JSON") {
      var re = /"(?:[^"\\]|\\.)*"|\b\d+\b|\btrue\b|\bfalse\b|\bnull\b/g;
      var m;
      while ((m = re.exec(line))) {
        var isKey = m[0][0] === '"' && /^\s*:/.test(line.slice(m.index + m[0].length));
        var cls = isKey ? "k" : m[0][0] === '"' ? "v" : "n";
        for (i = m.index; i < m.index + m[0].length; i++) out[i] = cls;
      }
    } else if (lang === "Go") {
      var cm = line.indexOf("//");
      var code = cm >= 0 ? line.slice(0, cm) : line;
      var re2 = /"(?:[^"\\]|\\.)*"|\b\d+\b|\b[A-Za-z_]\w*\b/g;
      var m2;
      while ((m2 = re2.exec(code))) {
        var w = m2[0];
        var c2 = w[0] === '"' ? "v" : /^\d/.test(w) ? "n" : KEYS.go.indexOf(w) >= 0 ? "k" : w === "string" || w === "int" ? "t" : "";
        if (c2) for (i = m2.index; i < m2.index + w.length; i++) out[i] = c2;
      }
      if (cm >= 0) for (i = cm; i < line.length; i++) out[i] = "c";
    }
    return out;
  }

  // expandTabs keeps a tab as one "\t" cell followed by blanks up to the next
  // multiple-of-4 column, like the editor's layout.
  function expandTabs(line) {
    if (line.indexOf("\t") < 0) return line;
    var out = "";
    for (var i = 0; i < line.length; i++) {
      if (line[i] === "\t") out += "\t" + "\u0000".repeat(3 - (out.length % 4));
      else out += line[i];
    }
    return out;
  }

  // wrapParts splits a line into screen rows like editor.Segments: after the
  // last space or tab that fits, else at the width. A line that fills its last
  // row exactly gets an empty extra row for the end-of-line cursor.
  function wrapParts(line, width) {
    var parts = [], from = 0;
    for (;;) {
      var rem = line.length - from;
      if (rem < width) { parts.push([from, line.length]); return parts; }
      if (rem === width) { parts.push([from, line.length], [line.length, line.length]); return parts; }
      var to = from + width;
      for (var j = from + width - 1; j > from; j--) {
        if (line[j] === " " || line[j] === "\t") { to = j + 1; break; }
      }
      parts.push([from, to]);
      from = to;
    }
  }

  // ---- one demo screen ----------------------------------------------------

  function Demo(el, def) {
    this.el = el;
    this.def = def;
    this.cols = def.cols || 52;
    // One more row than the definition says: the column ruler takes the first
    // text row, so the text keeps the height the scenario was written for.
    this.rows = (def.rows || 11) + (def.shell ? 0 : 1);
    this.host = el.querySelector(".screen") || el;
    this.caption = el.parentNode && el.parentNode.querySelector(".caption");
    this.keysEl = el.parentNode && el.parentNode.querySelector(".keystrip");
    this.token = null;
    this.reset();
  }

  Demo.prototype.reset = function () {
    this.s = {
      shell: false,
      shellLines: [],
      lines: [""],
      name: "[No Name]",
      lang: "Plain",
      colored: true,
      eol: "LF",
      bom: false,
      modified: false,
      invisibles: false,
      ruler: true, // column ruler (hidden while wrapping)
      wrap: false,
      split: false, // two stacked panes; top2 is the other pane's first line, focus2 tells the cursor is in the lower one
      top2: 0,
      focus2: false,
      left: 0, // horizontal scroll, in cells
      find: null, // live search: the text typed in the Find prompt
      cursors: [{ l: 0, c: 0 }],
      sels: [],
      pointer: null,
      ripple: null,
      guide: null,
      prompt: null, // {kind: "echo"|"input"|"error"|"choice", text, hint}
      popup: null, // {rows: [{t, hint}], sel}
      keyUntil: 0,
      top: 0
    };
    this.say("");
    if (this.keysEl) this.keysEl.innerHTML = "";
  };

  Demo.prototype.say = function (text) {
    if (this.caption) this.caption.textContent = text;
  };

  // press shows the key the person pressed, under the screen.
  Demo.prototype.press = function (label) {
    if (!this.keysEl) return;
    var parts = label.split("+");
    this.keysEl.innerHTML = parts.map(function (p) { return "<kbd>" + p + "</kbd>"; }).join(" + ");
    this.s.keyUntil = performance.now() + 1100;
  };

  Demo.prototype.sleep = function (ms) {
    var tok = this.token;
    return new Promise(function (resolve, reject) {
      if (!tok || tok.cancelled) return reject(CANCEL);
      var id = setTimeout(function () { tok.pending = null; resolve(); }, ms);
      tok.pending = function () { clearTimeout(id); reject(CANCEL); };
    });
  };

  // ---- state helpers for scenarios -----------------------------------------

  Demo.prototype.cur = function () { return this.s.cursors[0]; };

  Demo.prototype.type = async function (text, ms) {
    var s = this.s;
    for (var i = 0; i < text.length; i++) {
      var ch = text[i];
      var cs = s.cursors.slice().sort(function (a, b) { return b.l - a.l || b.c - a.c; });
      if (ch === "\n") {
        var p = s.cursors[0];
        var rest = s.lines[p.l].slice(p.c);
        s.lines[p.l] = s.lines[p.l].slice(0, p.c);
        s.lines.splice(p.l + 1, 0, rest);
        p.l++;
        p.c = 0;
      } else {
        for (var k = 0; k < cs.length; k++) {
          var cu = cs[k];
          var ln = s.lines[cu.l];
          s.lines[cu.l] = ln.slice(0, cu.c) + ch + ln.slice(cu.c);
          // a cursor on the same line to the right of this one moves too
          for (var j = 0; j < s.cursors.length; j++) {
            var o = s.cursors[j];
            if (o !== cu && o.l === cu.l && o.c > cu.c) o.c++;
          }
          cu.c++;
        }
      }
      s.modified = true;
      await this.sleep(ms);
    }
  };

  Demo.prototype.setLines = function (lines) { this.s.lines = lines.slice(); };
  Demo.prototype.cursor = function (l, c) { this.s.cursors = [{ l: l, c: c }]; };
  Demo.prototype.select = function (l, a, b) { this.s.sels = [{ l: l, a: a, b: b }]; };
  Demo.prototype.noSel = function () { this.s.sels = []; };

  Demo.prototype.click = function (l, c) {
    this.s.sels = [];
    this.s.cursors = [{ l: l, c: c }];
    this.rippleAt(l, c);
  };

  // textTop is the first text row: the ruler is above it while it shows.
  Demo.prototype.textTop = function () { return this.panes().list[this.focusIdx()].T; };
  Demo.prototype.focusIdx = function () { return this.s.split && this.panes().list.length > 1 && this.s.focus2 ? 1 : 0; };
  // panes mirrors app.regions: with a split, the text rows around one divider
  // row, the upper pane getting rows/2; each pane has its own ruler.
  Demo.prototype.panes = function () {
    var n = this.rows - 3, ru = this.rulerOn();
    function mk(y, rows) { return ru && rows >= 4 ? { ruler: y, T: y + 1, TR: rows - 1 } : { ruler: 0, T: y, TR: rows }; }
    if (this.s.split && !this.s.shell && n >= 9) {
      var up = n >> 1;
      return { list: [mk(1, up), mk(up + 2, n - 1 - up)], div: up + 1 };
    }
    return { list: [mk(1, n)], div: 0 };
  };
  Demo.prototype.rulerOn = function () { return !!(this.s.ruler && !this.s.wrap && !this.s.shell); };

  Demo.prototype.rippleAt = function (l, c) {
    this.s.ripple = { x: (this.s.gutter || GUTTER) + c - this.s.left, y: this.textTop() + l - this.s.top, t0: performance.now() };
  };

  Demo.prototype.echo = function (text, hint) { this.s.prompt = { kind: "echo", text: text, hint: hint || "" }; };
  Demo.prototype.input = function (label, text, note) { this.s.prompt = { kind: "input", text: label + text, hint: "", note: note || "" }; };
  // find sets the text of the live search: its other matches are underlined.
  Demo.prototype.find = function (q) { this.s.find = q || null; };
  Demo.prototype.error = function (text) { this.s.prompt = { kind: "error", text: text, hint: "" }; };
  Demo.prototype.choice = function (text) { this.s.prompt = { kind: "choice", text: text, hint: "" }; };
  Demo.prototype.idle = function () { this.s.prompt = null; this.s.popup = null; };
  Demo.prototype.popup = function (rows, sel) { this.s.popup = rows ? { rows: rows, sel: sel || 0 } : null; };

  // shell mode: lines of {t, cls}
  Demo.prototype.shell = function () { this.s.shell = true; this.s.shellLines = []; };
  Demo.prototype.sh = function (text, cls) { this.s.shellLines.push({ t: text, cls: cls || "" }); };
  Demo.prototype.typeShell = async function (cmd, ms) {
    var line = { t: "$ ", cls: "" };
    this.s.shellLines.push(line);
    for (var i = 0; i < cmd.length; i++) {
      line.t += cmd[i];
      await this.sleep(ms);
    }
  };

  // ---- drawing --------------------------------------------------------------

  Demo.prototype.render = function (now) {
    var W = this.cols, H = this.rows, s = this.s;
    var cells = [];
    var r, c;
    var G = s.gutter || GUTTER; // wider for files with many lines
    for (r = 0; r < H; r++) {
      cells.push([]);
      for (c = 0; c < W; c++) cells[r].push([" ", ""]);
    }
    function put(r, c, ch, cls) {
      if (r >= 0 && r < H && c >= 0 && c < W) cells[r][c] = [ch, cls];
    }
    function puts(r, c, str, cls) {
      for (var i = 0; i < str.length; i++) put(r, c + i, str[i], cls);
    }

    if (s.shell) {
      var lines = s.shellLines.slice(-H);
      for (r = 0; r < lines.length; r++) {
        // wrap long lines
        puts(r, 0, lines[r].t.slice(0, W), lines[r].cls);
      }
      return this.flush(cells);
    }

    // menu bar
    for (c = 0; c < W; c++) put(0, c, " ", "m");
    puts(0, 0, " File  Edit  Selection  Search  View  Help", "m");

    var width = W - G;
    var guide = s.guide && !s.wrap && now - s.guide.t0 < GUIDE_MS ? s.guide : null;
    var cur0 = s.cursors[0];
    var secondary = {};
    s.cursors.slice(1).forEach(function (p) { secondary[p.l + ":" + p.c] = true; });
    var lay = this.panes();
    var panes = lay.list, fi = this.focusIdx(), segsOf = [];
    panes.forEach(function (pn, pidx) {
      var T = pn.T, TR = pn.TR, top = pidx === fi ? s.top : (s.top2 || 0), segs;
      // column ruler: dots, and the number of every tenth column starting at that column
      if (pn.ruler) {
        for (c = 0; c < W; c++) put(pn.ruler, c, c < G ? " " : ".", "r");
        for (var col = (Math.max(s.left - 3, 0) / 10 | 0) * 10 + 10; col - s.left <= width; col += 10) {
          var num = String(col), rx = col - 1 - s.left;
          if (rx < 0) { num = num.slice(Math.min(-rx, num.length)); rx = 0; }
          puts(pn.ruler, G + rx, num.slice(0, Math.max(0, width - rx)), "r");
        }
      }
      // text rows: one segment per screen row (a whole line, or a piece of it while wrapping)
    segs = [];
    for (var sl0 = top; segs.length < TR && sl0 < s.lines.length; sl0++) {
      var sline = expandTabs(s.lines[sl0]);
      if (!s.wrap) {
        segs.push({ li: sl0, line: sline, from: s.left, to: s.left + width, first: true, last: true });
        continue;
      }
      var parts = wrapParts(sline, width);
      for (var pi = 0; pi < parts.length; pi++) {
        segs.push({ li: sl0, line: sline, from: parts[pi][0], to: parts[pi][1], first: pi === 0, last: pi === parts.length - 1 });
      }
    }
    var qlow = s.find ? (/[A-Z]/.test(s.find) ? s.find : s.find.toLowerCase()) : "";
    for (r = 0; r < TR; r++) {
      var y = T + r;
      var sg = r < segs.length ? segs[r] : null;
      var li = sg ? sg.li : -1;
      var line = sg ? sg.line : null;
      if (sg && sg.first) puts(y, 0, String(li + 1 + (s.base || 0)).padStart(G - 1, " ") + " ", li === cur0.l ? "gc" : "g");
      else if (sg) put(y, G - 2, "\u21b3", "g"); // continuation row of a wrapped line
      var cls = sg && s.colored ? classify(s.lang, line) : [];
      if (sg) {
        // control characters and invalid bytes are always visible
        var xm, xre = /\^A|�/g;
        while ((xm = xre.exec(line))) for (var xi = xm.index; xi < xm.index + xm[0].length; xi++) cls[xi] = "x";
        // the other matches of the live search (smart case, like Find)
        if (qlow) {
          var hay = /[A-Z]/.test(s.find) ? line : line.toLowerCase();
          for (var mi = hay.indexOf(qlow); mi >= 0; mi = hay.indexOf(qlow, mi + qlow.length)) {
            for (xi = mi; xi < mi + qlow.length; xi++) cls[xi] = "mt";
          }
        }
      }
      for (c = 0; c < width; c++) {
        var ix = sg ? sg.from + c : -1;
        var ch = sg && ix < line.length && ix < sg.to ? line[ix] : null;
        if (ch === null) {
          put(y, G + c, sg && sg.last ? "·" : sg ? " " : "·", sg && sg.last || !sg ? "e" : "");
        } else if (s.invisibles && ch === " ") {
          put(y, G + c, "␣", "e");
        } else if (ch === "\u0000") {
          put(y, G + c, " ", "");
        } else if (ch === "\t") {
          put(y, G + c, s.invisibles ? "»" : " ", s.invisibles ? "e" : "");
        } else {
          put(y, G + c, ch, cls[ix] || "");
        }
      }
      if (!sg) continue;
      var reach = sg.last ? sg.from + width : sg.to; // cells this row can show
      s.sels.forEach(function (sl) {
        if (sl.l !== li) return;
        var b = sl.b < 0 ? line.length + 1 : sl.b;
        for (var cc = Math.max(sl.a, sg.from); cc < Math.min(b, reach); cc++) {
          put(y, G + cc - sg.from, cc < line.length ? (line[cc] === " " ? " " : line[cc]) : " ", "sel");
        }
      });
      for (var key in secondary) {
        var pr = key.split(":");
        if (+pr[0] === li && +pr[1] >= sg.from && +pr[1] < reach) put(y, G + +pr[1] - sg.from, +pr[1] < line.length ? line[+pr[1]] : " ", "cur2");
      }
    }

      segsOf.push(segs);
    });
    if (lay.div) for (c = 0; c < W; c++) put(lay.div, c, " ", "dv");
    var T = panes[fi].T, TR = panes[fi].TR, segs = segsOf[fi];
    // ripple, then the indent guide, over the text
    if (s.ripple) {
      var age = now - s.ripple.t0;
      if (age >= RIPPLE_MS) s.ripple = null;
      else {
        for (r = T; r < T + TR; r++) {
          for (c = 0; c < W; c++) {
            var lv = rippleLevel(c - s.ripple.x, r - s.ripple.y, age);
            if (lv) cells[r][c][1] = (cells[r][c][1] + " t" + lv).trim();
          }
        }
      }
    }
    if (guide) {
      var gx = G + guide.col;
      for (r = 0; r < TR; r++) {
        var gl = s.top + r < s.lines.length ? s.lines[s.top + r] : null;
        var gch = gl !== null && guide.col < gl.length ? gl[guide.col] : " ";
        if (gl !== null && indentCol(gl) === guide.col) put(T + r, gx, gch, "gm");
        else if (gl !== null && gch !== " ") cells[T + r][gx][1] += " t4";
        else put(T + r, gx, "|", "gd");
      }
    }

    if (s.pointer) {
      var pl = s.pointer.l - s.top;
      if (pl >= 0 && pl < TR) cells[T + pl][G + s.pointer.c][1] += " ptr";
    }

    // the cursor is a block, hidden while the prompt has the focus
    if (!s.sels.length && !(s.prompt && s.prompt.kind === "input")) {
      var cl = cur0.l < s.lines.length ? expandTabs(s.lines[cur0.l]) : "";
      for (r = 0; r < segs.length; r++) {
        var cg = segs[r];
        if (cg.li === cur0.l && cur0.c >= cg.from && (cur0.c < cg.to || cg.last)) {
          put(T + r, G + cur0.c - cg.from, cur0.c < cl.length ? cl[cur0.c] : " ", "cur");
          break;
        }
      }
    }

    // completion popup above the prompt line
    var py = H - 2;
    if (s.popup) {
      var n = s.popup.rows.length;
      for (var i = 0; i < n; i++) {
        var row = s.popup.rows[i];
        var rc = i === s.popup.sel ? "psel" : "pop";
        var ry = py - n + i;
        for (c = 0; c < W; c++) put(ry, c, " ", rc);
        puts(ry, 2, row.t, rc);
        if (row.hint) puts(ry, W - row.hint.length - 1, row.hint, rc);
      }
    }

    // prompt line
    var p = s.prompt;
    var pcls = !p ? "pi" : p.kind === "error" ? "perr" : "p";
    for (c = 0; c < W; c++) put(py, c, " ", pcls);
    if (!p) {
      puts(py, 0, "write a command…", "pi");
      puts(py, W - 11, "Ctrl+Alt+P", "pi");
    } else if (p.kind === "echo") {
      puts(py, 0, "> " + p.text, "p");
      if (p.hint) puts(py, W - p.hint.length - 1, p.hint, "p");
    } else if (p.kind === "input") {
      puts(py, 0, p.text, "p");
      var cx = p.text.length;
      put(py, cx, " ", "cur");
      if (p.note) puts(py, cx + 1, " " + p.note, "perr");
    } else {
      puts(py, 0, p.text.length > W ? p.text.slice(0, W - 1) + "…" : p.text, pcls);
    }

    // status line
    for (c = 0; c < W; c++) put(H - 1, c, " ", "s");
    var left = "sie v0.9.0 - " + s.name + (s.modified ? " [modified]" : "");
    var pos = "Ln " + (cur0.l + 1 + (s.base || 0)) + ", Col " + (cur0.c + 1) + (s.cursors.length > 1 ? " (" + s.cursors.length + " cursors)" : "") + (s.wrap ? "   WRAP" : "");
    var right = pos + "   " + s.lang + "   UTF-8" + (s.bom ? " BOM" : "") + "   " + s.eol;
    var room = W - right.length - 2;
    if (left.length > room) left = left.slice(0, Math.max(0, room - 1)) + "…";
    puts(H - 1, 0, left, "s");
    puts(H - 1, W - right.length - 1, right, "s");

    this.flush(cells);
  };

  Demo.prototype.flush = function (cells) {
    var out = [];
    for (var r = 0; r < cells.length; r++) {
      var html = "";
      var run = null;
      for (var c = 0; c < cells[r].length; c++) {
        var cl = cells[r][c][1];
        if (cl !== run) {
          if (run !== null) html += "</span>";
          html += '<span class="' + cl + '">';
          run = cl;
        }
        html += esc(cells[r][c][0]);
      }
      out.push(html + "</span>");
    }
    this.host.innerHTML = '<pre aria-hidden="true">' + out.join("\n") + "</pre>";
    if (this.keysEl && this.s.keyUntil && performance.now() > this.s.keyUntil) {
      this.keysEl.innerHTML = "";
      this.s.keyUntil = 0;
    }
  };

  // ---- running --------------------------------------------------------------

  Demo.prototype.start = function () {
    if (this.token && !this.token.cancelled) return;
    var self = this;
    var tok = { cancelled: false, pending: null };
    this.token = tok;
    (async function () {
      try {
        while (!tok.cancelled) {
          self.reset();
          await self.def.run(self);
          await self.sleep(1200);
        }
      } catch (e) {
        if (e !== CANCEL) throw e;
      }
    })();
  };

  Demo.prototype.stop = function () {
    if (!this.token) return;
    this.token.cancelled = true;
    if (this.token.pending) this.token.pending();
    this.token = null;
  };

  Demo.prototype.drawStill = function () {
    this.reset();
    this.def.still(this);
    this.render(performance.now());
  };

  // ---- the page-level manager ----------------------------------------------

  var active = [];
  var looping = false;

  function loop() {
    if (!active.length) { looping = false; return; }
    var now = performance.now();
    for (var i = 0; i < active.length; i++) active[i].render(now);
    requestAnimationFrame(loop);
  }

  function activate(d) {
    if (active.indexOf(d) < 0) active.push(d);
    d.start();
    if (!looping) { looping = true; requestAnimationFrame(loop); }
  }

  function deactivate(d) {
    var i = active.indexOf(d);
    if (i >= 0) active.splice(i, 1);
    d.stop();
  }

  window.SieScreens = {
    CANCEL: CANCEL,
    Demo: Demo,
    init: function (defs) {
      var reduced = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
      var nodes = document.querySelectorAll("[data-demo]");
      var demos = [];
      nodes.forEach(function (el) {
        var def = defs[el.getAttribute("data-demo")];
        if (!def) return;
        var d = new Demo(el, def);
        d.el.__demo = d;
        demos.push(d);
        if (reduced) d.drawStill();
      });
      if (reduced || !("IntersectionObserver" in window)) {
        if (!reduced) demos.forEach(function (d) { d.drawStill(); });
        return demos;
      }
      var io = new IntersectionObserver(function (entries) {
        entries.forEach(function (e) {
          var d = e.target.__demo;
          if (e.isIntersecting) activate(d); else deactivate(d);
        });
      }, { threshold: 0.35 });
      demos.forEach(function (d) { d.drawStill(); io.observe(d.el); });
      document.addEventListener("visibilitychange", function () {
        if (document.hidden) active.slice().forEach(function (d) { d.stop(); });
        else demos.forEach(function (d) {
          var r = d.el.getBoundingClientRect();
          if (r.bottom > 0 && r.top < innerHeight) activate(d);
        });
      });
      return demos;
    }
  };
})();
