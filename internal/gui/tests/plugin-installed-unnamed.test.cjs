// Run with Node's test runner and Playwright on the module path; see README.md.
// A plugin installed a moment ago has no subscriptions named for it yet: the
// Installed list is drawn from what the plugins said the last time magpie asked
// them, which a plugin installed since hasn't been asked about yet, and asking
// them means every plugin's vendor answering, one at a time.
//
// So the row once said "Signs in to nothing magpie can use" for a plugin that
// does sign in to something — a claim the page had no way of knowing, said for
// as long as the answer took to come. It says nothing instead, and the line is
// there once the names have come. No backend here: the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const HOME = "/Users/aimer";
const NEW = "opencode-brandnew-auth";
const OLD = "opencode-copilot-auth";
const claude = { id: "claude", name: "Claude Code", icon: "", skills: `${HOME}/.claude/skills`, mcp: `${HOME}/.claude/mcp.json` };
const lib = {
  dir: `${HOME}/.magpie/library`, backups: `${HOME}/.magpie/backups`, home: HOME,
  agents: [claude],
  instructions: { agents: [], sets: [] }, foundServers: [], projects: [], foundSkills: [], skills: [], servers: [],
};

function server(lang, state) {
  return async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return route.fulfill({ json: { agents: [], profiles: [], settings: { lang, theme: "light" } } });
    if (url.pathname === "/api/library") return route.fulfill({ json: lib });
    if (url.pathname === "/api/usage/quotas") return route.fulfill({ json: [] });
    if (url.pathname === "/api/groups") return route.fulfill({ json: { groups: [] } });
    if (url.pathname === "/api/providers") return route.fulfill({ json: state.providers });
    if (url.pathname === "/api/plugins") return route.fulfill({ json: state.plugins });
    if (url.pathname.startsWith("/api/")) return route.fulfill({ json: {} });
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const words = {
  en: { signsIn: "Signs in to", nothing: "Signs in to nothing magpie can use" },
  zh: { signsIn: "可登录", nothing: "没有 magpie 能用的登录" },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": a plugin whose subscriptions are not named yet claims nothing", async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    const errors = [];
    t.after(async () => {
      if (errors.length) console.log(errors);
      await browser.close();
    });
    for (const lang of ["en", "zh"]) {
      await t.test(lang, async () => {
        const w = words[lang];
        const state = {
          // the plugin installed a moment ago: it is in the list, and the
          // providers answer names nothing for it yet
          plugins: {
            plugins: [
              { spec: OLD, providers: ["GitHub Copilot"], version: "0.1.0", moved: [] },
              { spec: NEW, providers: [], version: "0.1.0", moved: [] },
            ],
            bun: true, bunVersion: "1.3.0", picker: false, mirror: false,
          },
          providers: { providers: [], gateway: { running: true } },
        };
        const ctx = await browser.newContext({ viewport: { width: 980, height: 800 } });
        await ctx.addInitScript(() => { try { localStorage.setItem("magpie.pluginTab", "installed"); } catch {} });
        const page = await ctx.newPage();
        page.setDefaultTimeout(5000);
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", server(lang, state));
        await page.goto("http://magpie.test/");
        await page.locator('button[data-view="plugins"]').click();
        const v = page.locator("#view-plugins");
        await v.locator(".pm-row", { hasText: NEW }).waitFor();

        const body = await v.innerText();
        assert(!body.includes(w.nothing), "no row claims a plugin signs in to nothing:\n" + body);
        // the one whose names have come still says what it signs in to
        const old = v.locator(".pm-row", { hasText: OLD });
        assert((await old.innerText()).includes(w.signsIn), "the known plugin still names its subscriptions:\n" + (await old.innerText()));
        await ctx.close();
      });
    }
    assert.deepEqual(errors, []);
  });
}
