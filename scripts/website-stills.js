// Prints (or, with --write, patches into website/index.html) the no-JS
// fallback of every demo: the plain text of its still() frame.
//
//   node scripts/website-stills.js           # show which fallbacks are stale
//   node scripts/website-stills.js --write   # refresh them
//
// It runs the real website/sie-screen.js and website/demos.js against a stub
// page, so the fallbacks can never drift from the engine.
"use strict";
const fs = require("fs");
const path = require("path");
const vm = require("vm");

const root = path.join(__dirname, "..", "website");
const indexPath = path.join(root, "index.html");

const ctx = {
  console,
  performance: { now: () => 1e6 },
  requestAnimationFrame: () => 0,
  setTimeout,
  clearTimeout,
};
ctx.window = ctx;
ctx.document = { querySelectorAll: () => [], addEventListener() {} };
vm.createContext(ctx);
vm.runInContext(fs.readFileSync(path.join(root, "sie-screen.js"), "utf8"), ctx);

let defs = null;
ctx.SieScreens.init = (d) => { defs = d; return []; };
vm.runInContext(fs.readFileSync(path.join(root, "demos.js"), "utf8"), ctx);

const unescape = (s) => s.replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&amp;/g, "&");

function still(id) {
  const host = { innerHTML: "" };
  const el = { querySelector: () => host, parentNode: null };
  const d = new ctx.SieScreens.Demo(el, defs[id]);
  d.drawStill();
  const pre = host.innerHTML.replace(/^<pre[^>]*>/, "").replace(/<\/pre>$/, "");
  return pre.replace(/<[^>]+>/g, "").split("\n").map((l) => l.replace(/\s+$/, "")).join("\n");
}

let html = fs.readFileSync(indexPath, "utf8");
let stale = 0;
for (const id of Object.keys(defs)) {
  const re = new RegExp('(data-demo="' + id + '"[^>]*><div class="screen"><pre>)([\\s\\S]*?)(</pre>)');
  const m = re.exec(html);
  if (!m) { console.log("no fallback in index.html for", id); stale++; continue; }
  const want = still(id);
  const have = m[2].split("\n").map((l) => l.replace(/\s+$/, "")).join("\n");
  if (want !== have) {
    stale++;
    console.log("stale:", id);
    if (process.argv.includes("--write")) html = html.replace(re, (_, a, _b, c) => a + want + c);
  }
}
if (process.argv.includes("--write")) fs.writeFileSync(indexPath, html);
else if (stale) process.exit(1);
