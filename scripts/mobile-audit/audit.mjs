// Layout audit for the web UI, driven over the Chrome DevTools Protocol.
//
// Renders every page in PAGES at a given viewport (phone emulation by
// default), saves a full-page screenshot, and reports what a stylesheet
// cannot see about itself: horizontal overflow of the document, tables wider
// than their container that have no scrollable ancestor (and so are clipped
// by `.container { overflow: hidden }`), the date-picker dropdown leaving the
// viewport, tap targets under 32px, and text under 12px.
//
// No Go test can observe client-side layout, so this is the regression test
// for the CSS and template work. See run.sh for the phone + desktop pair and
// the desktop screenshot diff against a baseline.
//
//   BASE=http://127.0.0.1:8080 OUT=./out WIDTH=390 MOBILE=1 node audit.mjs
//
// Needs Node 22 (built-in WebSocket + fetch) and /usr/bin/chromium.

import { spawn } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';

const BASE = process.env.BASE || 'http://127.0.0.1:8080';
const OUT = process.env.OUT || './out';
const WIDTH = +(process.env.WIDTH || 390);
const HEIGHT = +(process.env.HEIGHT || (WIDTH < 800 ? 844 : 900));
const MOBILE = (process.env.MOBILE ?? (WIDTH < 800 ? '1' : '0')) === '1';
const CHROME = process.env.CHROME || '/usr/bin/chromium';
const SEARCH = (process.env.SEARCH ?? '1') === '1';
const MAX_SHOT = +(process.env.MAX_SHOT || 9000);

// Every page template, with parameters that resolve on the production data
// set. A page whose parameters stop resolving shows up as a non-200 status in
// the report rather than silently vanishing from the audit.
const DEFAULT_PAGES = [
  '/', '/stats', '/browse', '/browse/zone/2', '/browse/region/2/50', '/browse/net/2/5001',
  '/node/2/5001/100', '/points/2/5001/3/1', '/reachability/test?zone=21&net=3&node=220&time=2026-10-15T21:09:02Z', '/reachability/node?zone=21&net=3&node=220',
  '/analytics/other-networks/nodes?network=fsxnet',
  '/analytics', '/analytics/binkp', '/analytics/ifcico', '/analytics/telnet', '/analytics/vmodem',
  '/analytics/vmodem-unavailable', '/analytics/ftp', '/analytics/email', '/analytics/pingtrace',
  '/analytics/pingtrace/node?address=2:5001/100', '/analytics/ipv6', '/analytics/ipv6-nonworking',
  '/analytics/ipv6-advertised-ipv4-only', '/analytics/ipv6-only', '/analytics/pure-ipv6-only',
  '/analytics/ipv6-weekly-news', '/analytics/ipv6-node-list', '/analytics/aka-mismatch',
  '/analytics/other-networks', '/analytics/pstn', '/analytics/pstn-accessible',
  '/analytics/pstn-no-answer', '/analytics/file-request', '/analytics/software/binkp',
  '/analytics/software/ifcico', '/analytics/geo-hosting', '/analytics/geo-hosting/country?code=US',
  '/analytics/pioneers', '/analytics/domain-expiration', '/analytics/registrars',
  '/analytics/on-this-day', '/analytics/flag', '/analytics/network',
  '/reachability', '/reachability/node?address=2:5001/100',
  '/nodelists', '/nodelists/fidonet/2026', '/pointlists', '/pointlists/fidonet/z2/2025',
  '/links', '/api/help',
];
const PAGES = process.env.PAGES ? process.env.PAGES.split(',') : DEFAULT_PAGES;

mkdirSync(OUT, { recursive: true });

const port = 9300 + Math.floor(Math.random() * 500);
const chrome = spawn(CHROME, ['--headless=new', `--remote-debugging-port=${port}`, '--no-sandbox',
  '--disable-gpu', '--hide-scrollbars', `--window-size=${WIDTH},${HEIGHT}`, 'about:blank'], { stdio: 'ignore' });
await new Promise(r => setTimeout(r, 1500));
const ver = await (await fetch(`http://127.0.0.1:${port}/json/version`)).json();
const ws = new WebSocket(ver.webSocketDebuggerUrl);
await new Promise(r => (ws.onopen = r));
let id = 0; const pending = new Map(); const events = [];
ws.onmessage = e => { const m = JSON.parse(e.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); } else events.push(m); };
const send = (method, params = {}, sessionId) => new Promise(res => { const i = ++id; pending.set(i, res); ws.send(JSON.stringify({ id: i, method, params, sessionId })); });

const { result: { targetId } } = await send('Target.createTarget', { url: 'about:blank' });
const { result: { sessionId } } = await send('Target.attachToTarget', { targetId, flatten: true });
const S = (m, p) => send(m, p, sessionId);
await S('Page.enable'); await S('Runtime.enable'); await S('Network.enable');
await S('Emulation.setDeviceMetricsOverride', { width: WIDTH, height: HEIGHT, deviceScaleFactor: MOBILE ? 2 : 1, mobile: MOBILE });
if (MOBILE) {
  await S('Emulation.setTouchEmulationEnabled', { enabled: true });
  await S('Network.setUserAgentOverride', { userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1' });
}

// Runs in the page. Returns JSON.
const AUDIT = `(() => {
  const vw = window.innerWidth;
  const sel = el => el.tagName.toLowerCase() + ((el.className && typeof el.className === 'string') ? '.' + el.className.trim().split(/\\s+/).slice(0,2).join('.') : '');
  const wide = [];
  for (const el of document.querySelectorAll('body *')) {
    const r = el.getBoundingClientRect();
    if (r.width > vw + 2 || r.right > vw + 2) wide.push(sel(el) + ' ' + Math.round(r.width) + 'px');
  }
  const small = [];
  for (const el of document.querySelectorAll('a,button,select,input,[role=button]')) {
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) continue;
    if (r.height < 32 || r.width < 32) small.push(sel(el) + ' ' + Math.round(r.width) + 'x' + Math.round(r.height) + ' "' + (el.textContent||el.value||'').trim().slice(0,20) + '"');
  }
  let tiny = 0;
  for (const el of document.querySelectorAll('body *')) {
    if (el.children.length || !el.textContent.trim()) continue;
    if (parseFloat(getComputedStyle(el).fontSize) < 12) tiny++;
  }
  const tables = [];
  const clipped = [];
  for (const t of document.querySelectorAll('table')) {
    let el = t.parentElement, anc = null;
    while (el && el !== document.body) { const ox = getComputedStyle(el).overflowX; if (ox === 'auto' || ox === 'scroll') { anc = el; break; } el = el.parentElement; }
    const cols = t.querySelectorAll('thead th').length;
    const w = Math.round(t.getBoundingClientRect().width);
    if (!anc) {
      const pw = Math.round(t.parentElement.getBoundingClientRect().width);
      const desc = cols + ' cols ' + w + 'px, no scroll ancestor, parent ' + pw + 'px';
      tables.push(desc);
      if (w > pw + 2) clipped.push(desc);
    } else {
      tables.push(cols + ' cols ' + w + 'px in ' + sel(anc) + ' (' + anc.clientWidth + '/' + anc.scrollWidth + (anc.scrollWidth > anc.clientWidth + 1 ? ', scrolls)' : ')'));
    }
  }
  // Date picker: open it and see whether it stays inside the viewport.
  let picker = null;
  const toggle = document.querySelector('.date-picker-toggle');
  const dd = document.querySelector('.date-picker-dropdown');
  if (toggle && dd) {
    toggle.scrollIntoView({ block: 'center' });
    toggle.click();
    const r = dd.getBoundingClientRect();
    const open = getComputedStyle(dd).display !== 'none';
    picker = { open, left: Math.round(r.left), right: Math.round(r.right), top: Math.round(r.top), bottom: Math.round(r.bottom), vw, vh: window.innerHeight,
               // Below the fold is fine (the page scrolls); off the sides or above the top is not.
               ok: open && r.left >= 0 && r.right <= vw && r.top >= 0 };
    toggle.click();
    window.scrollTo(0, 0);
  }
  return JSON.stringify({ vw, scrollWidth: document.documentElement.scrollWidth, docHeight: document.documentElement.scrollHeight,
    overflow: document.documentElement.scrollWidth - vw, wide: wide.slice(0, 8), wideCount: wide.length,
    small: small.slice(0, 6), smallCount: small.length, tinyTextNodes: tiny, tables, clipped, picker });
})()`;

const report = []; let failures = 0;
async function auditCurrentPage(label, status) {
  const { result } = await S('Runtime.evaluate', { expression: AUDIT, returnByValue: true });
  const a = JSON.parse(result.result.value);
  const fname = OUT + '/' + (label.replace(/[^a-z0-9]+/gi, '_').replace(/^_|_$/g, '') || 'home') + '.png';
  const h = Math.min(a.docHeight, MAX_SHOT);
  const shot = await S('Page.captureScreenshot', { format: 'png', captureBeyondViewport: true, clip: { x: 0, y: 0, width: WIDTH, height: h, scale: 1 } });
  writeFileSync(fname, Buffer.from(shot.result.data, 'base64'));
  const problems = [];
  if (status !== 200) problems.push('status ' + status);
  if (a.overflow > 0) problems.push('document overflows by ' + a.overflow + 'px');
  for (const c of a.clipped) problems.push('clipped table: ' + c);
  if (a.picker && !a.picker.ok) problems.push('date picker outside viewport: ' + JSON.stringify(a.picker));
  if (problems.length) failures++;
  report.push({ page: label, status, file: fname, problems, ...a });
  console.log(`\n=== ${label}  [${status}] height ${a.docHeight}px${problems.length ? '  ** ' + problems.length + ' PROBLEM(S)' : ''}`);
  for (const p of problems) console.log('  !! ' + p);
  console.log(`  overflow ${a.overflow}px | wider-than-viewport elements ${a.wideCount}${a.wide.length ? ': ' + a.wide.slice(0, 4).join(' | ') : ''}`);
  if (a.tables.length) console.log('  tables: ' + a.tables.join(' ; '));
  console.log(`  tap targets <32px: ${a.smallCount}${a.small.length ? '  e.g. ' + a.small.slice(0, 3).join(' | ') : ''} | text <12px: ${a.tinyTextNodes}`);
}

async function waitLoaded() {
  for (let i = 0; i < 300; i++) { await new Promise(r => setTimeout(r, 100)); if (events.some(e => e.method === 'Page.loadEventFired')) break; }
  const doc = events.find(e => e.method === 'Network.responseReceived' && e.params.type === 'Document');
  events.length = 0;
  await new Promise(r => setTimeout(r, 1200));
  return doc ? doc.params.response.status : 0;
}

for (const p of PAGES) {
  await S('Page.navigate', { url: BASE + p });
  const status = await waitLoaded();
  await auditCurrentPage(p, status);
}

if (SEARCH) {
  // The search form is POST-only; submit a sysop search for a results table.
  await S('Page.navigate', { url: BASE + '/' }); await waitLoaded();
  await S('Runtime.evaluate', { expression: `document.querySelector("input[name=sysop_name]").value="Protasov"; document.querySelector("form.search-form").submit();` });
  const status = await waitLoaded();
  await auditCurrentPage('POST search sysop=Protasov', status || 200);
}

writeFileSync(OUT + '/report.json', JSON.stringify(report, null, 2));
console.log(`\n${report.length} pages, ${failures} with problems. Screenshots and report.json in ${OUT}`);
chrome.kill();
process.exit(failures && process.env.FAIL === '1' ? 1 : 0);
