// Behaviour checks that a screenshot cannot show, driven over CDP in phone
// emulation: a table header click sorts by that column (sortable-table.js),
// a tap on an IP tooltip opens it as an on-screen sheet (tooltip.js), the
// hidden-cell count of the sticky first column, and the nav strip scrolls.
//
//   BASE=http://127.0.0.1:8080 node interact.mjs

import { spawn } from 'node:child_process';

const BASE = process.env.BASE || 'http://127.0.0.1:8080';
const CHROME = process.env.CHROME || '/usr/bin/chromium';
const port = 9800 + Math.floor(Math.random() * 100);
const chrome = spawn(CHROME, ['--headless=new', `--remote-debugging-port=${port}`, '--no-sandbox', '--disable-gpu', 'about:blank'], { stdio: 'ignore' });
await new Promise(r => setTimeout(r, 1500));
const ver = await (await fetch(`http://127.0.0.1:${port}/json/version`)).json();
const ws = new WebSocket(ver.webSocketDebuggerUrl);
await new Promise(r => (ws.onopen = r));
let id = 0; const pending = new Map();
ws.onmessage = e => { const m = JSON.parse(e.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); } };
const send = (method, params = {}, sessionId) => new Promise(res => { const i = ++id; pending.set(i, res); ws.send(JSON.stringify({ id: i, method, params, sessionId })); });
const { result: { targetId } } = await send('Target.createTarget', { url: 'about:blank' });
const { result: { sessionId } } = await send('Target.attachToTarget', { targetId, flatten: true });
const S = (m, p) => send(m, p, sessionId);
await S('Page.enable'); await S('Runtime.enable');
await S('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
await S('Emulation.setTouchEmulationEnabled', { enabled: true });
const go = async p => { await S('Page.navigate', { url: BASE + p }); await new Promise(r => setTimeout(r, 3000)); };
const ev = async expr => { const { result } = await S('Runtime.evaluate', { expression: expr, returnByValue: true, awaitPromise: true }); return result.result.value; };
const tap = async (x, y) => {
  await S('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] });
  await S('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await new Promise(r => setTimeout(r, 400));
};
let failures = 0;
const check = (name, ok, detail) => { console.log(`${ok ? 'ok  ' : 'FAIL'} ${name}${detail ? '  (' + detail + ')' : ''}`); if (!ok) failures++; };

// 1. Sorting by the clicked column, on a table that has a non-sortable
//    column before a sortable one (the shape the old index bug mis-sorted).
await go('/analytics/email');
const sortInfo = await ev(`(() => {
  const t = document.querySelector('table.sortable-table'); if (!t) return null;
  const hs = [...t.querySelectorAll('thead th')];
  const target = hs.find((h, i) => h.hasAttribute('data-sortable') && i > 0 && !hs[i-1].hasAttribute('data-sortable'));
  if (!target) return { none: true };
  target.click(); const order = target.getAttribute('data-order');
  const idx = target.cellIndex;
  const vals = [...t.querySelectorAll('tbody tr')].map(r => (r.cells[idx] ? r.cells[idx].textContent.trim() : '')).slice(0, 12);
  const sorted = [...vals].sort((a, b) => a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' }));
  if (order === 'desc') sorted.reverse();
  return { header: target.textContent.trim(), order, idx, vals, inOrder: JSON.stringify(vals) === JSON.stringify(sorted), ariaSort: target.getAttribute('aria-sort') };
})()`);
check('header click sorts its own column (/analytics/email)', sortInfo && (sortInfo.none || sortInfo.inOrder), sortInfo && (sortInfo.none ? 'no such table shape' : `${sortInfo.header} col ${sortInfo.idx} ${sortInfo.order}, aria-sort=${sortInfo.ariaSort}`));

// 2. Tap an IP tooltip: it must open and sit inside the viewport.
await go('/analytics/binkp');
const tip = await ev(`(() => {
  const el = document.querySelector('.ip-tooltip, .ipv6-tooltip'); if (!el) return null;
  // Scroll the window (not the table's own scroll box) so the trigger sits
  // mid-screen; the stylesheet asks for smooth scrolling, so insist on an
  // instant one and give layout a moment before measuring.
  window.scrollTo({ top: window.scrollY + el.getBoundingClientRect().top - 300, behavior: 'instant' });
  return new Promise(res => setTimeout(() => { const r = el.getBoundingClientRect();
    res({ x: r.left + Math.min(20, r.width / 2), y: r.top + r.height / 2, role: el.getAttribute('role'), tab: el.getAttribute('tabindex') }); }, 300));
})()`);
if (!tip) check('ip tooltip present on /analytics/binkp', false);
else {
  await tap(tip.x, tip.y);
  const open = await ev(`(() => {
    const el = document.querySelector('.ip-tooltip.is-open, .ipv6-tooltip.is-open'); if (!el) return { open: false };
    const cs = getComputedStyle(el, '::after');
    return { open: true, position: cs.position, left: parseFloat(cs.left), right: parseFloat(cs.right), pe: cs.pointerEvents, expanded: el.getAttribute('aria-expanded') };
  })()`);
  check('tap opens the tooltip as a fixed sheet', open.open && open.position === 'fixed' && open.pe === 'auto', JSON.stringify(open));
  await tap(200, 100);
  const closed = await ev(`!document.querySelector('.is-open[data-tooltip-toggle]')`);
  check('tap elsewhere closes it', closed);
  check('tooltip trigger is focusable with a role', tip.role === 'button' && tip.tab === '0');
}

// 3. Sticky first column and the scroll affordance on a wide table.
const sticky = await ev(`(() => {
  const t = document.querySelector('table.table--sticky-first'); if (!t) return null;
  const td = t.querySelector('tbody tr td:first-child'); const cs = getComputedStyle(td);
  const wrap = t.closest('.table-scroll'); const inner = t.closest('.table-responsive, .table-container');
  const before = inner && (inner.scrollLeft = 120, inner.dispatchEvent(new Event('scroll')), true);
  return { position: cs.position, moreRight: wrap ? wrap.hasAttribute('data-more-right') : null, moreLeftAfterScroll: wrap ? wrap.hasAttribute('data-more-left') : null, hint: !!document.querySelector('.table-scroll__hint'), sortableHeaderPosition: getComputedStyle(t.querySelector('thead th[data-sortable]')).position };
})()`);
check('first column is sticky and the wrapper tracks scroll state', sticky && sticky.position === 'sticky' && sticky.moreRight && sticky.moreLeftAfterScroll, JSON.stringify(sticky));

// 4. The nav strip scrolls horizontally and the network row sits below it.
await go('/analytics');
const nav = await ev(`(() => {
  const strip = document.querySelector('.nav-links'); const net = document.querySelector('.nav-network');
  if (!strip) return null; const r = strip.getBoundingClientRect(); const n = net ? net.getBoundingClientRect() : null;
  return { scrolls: strip.scrollWidth > strip.clientWidth, display: getComputedStyle(strip).display, stripBottom: Math.round(r.bottom), netTop: n ? Math.round(n.top) : null, height: Math.round(r.height) };
})()`);
check('nav pills are one scrolling row with the network select below', nav && nav.scrolls && nav.display === 'flex' && (nav.netTop === null || nav.netTop >= nav.stripBottom), JSON.stringify(nav));

// 5. Date picker sheet closes on Escape and returns focus.
await go('/stats');
const dp = await ev(`(async () => {
  const toggle = document.querySelector('.date-picker-toggle'); const dd = document.querySelector('.date-picker-dropdown');
  if (!toggle || !dd) return null; toggle.click(); await new Promise(r => setTimeout(r, 100));
  const r = dd.getBoundingClientRect(); const open = getComputedStyle(dd).display !== 'none';
  const closeBtn = dd.querySelector('.date-picker-close'); const closeVisible = closeBtn && getComputedStyle(closeBtn).display !== 'none';
  const overflowing = [...dd.querySelectorAll('button, select')].filter(c => { const b = c.getBoundingClientRect(); return b.width && (b.right > r.right + 1 || b.left < r.left - 1); }).length;
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await new Promise(r => setTimeout(r, 100));
  return { open, inView: r.left >= 0 && r.right <= innerWidth && r.bottom <= innerHeight, closeVisible, controlsOverflowing: overflowing, closedByEscape: getComputedStyle(dd).display === 'none', focusBack: document.activeElement === toggle };
})()`);
check('date picker opens as an on-screen sheet with a close button, Escape closes it', dp && dp.open && dp.inView && dp.closeVisible && dp.controlsOverflowing === 0 && dp.closedByEscape && dp.focusBack, JSON.stringify(dp));

console.log(`\n${failures} failure(s)`);
chrome.kill();
process.exit(failures ? 1 : 0);
