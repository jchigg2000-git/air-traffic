// Capture the Gateway Harness page (raw + numbered-callout annotated) for docs/images/.
// The callout numbers must stay in sync with the legend in docs/inference-gateway-eli5.md #6.
// Prereqs: compose stack up (control plane on :8122 with a fresh gateway heartbeat and at
// least one completed harness run), plus `npm i playwright-core` and a Playwright-cached
// Chromium (~/Library/Caches/ms-playwright).
// Usage: node scripts/capture-harness-screenshot.js docs/images
const path = require('path');
const os = require('os');
const fs = require('fs');
const { chromium } = require('playwright-core');

const OUT = process.argv[2] || '.';
const BASE = 'http://localhost:8122';

function chromePath() {
  const root = path.join(os.homedir(), 'Library/Caches/ms-playwright');
  const cands = fs.readdirSync(root).filter(d => /^chromium/.test(d)).sort().reverse();
  for (const c of cands) {
    for (const rel of [
      'chrome-mac-arm64/Chromium.app/Contents/MacOS/Chromium',
      'chrome-headless-shell-mac-arm64/chrome-headless-shell',
    ]) {
      const p = path.join(root, c, rel);
      if (fs.existsSync(p)) return p;
    }
  }
  throw new Error('no cached chromium found');
}

(async () => {
  const browser = await chromium.launch({ executablePath: chromePath() });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, deviceScaleFactor: 2 });
  await page.goto(BASE + '/settings/harness', { waitUntil: 'networkidle' });
  await page.waitForTimeout(1500);

  // Populate the try-a-prompt box with a real round trip.
  const ta = page.locator('textarea');
  await ta.fill('Draft a reply to Jane Doe, SSN 123-45-6789, callback 555.123.4567');
  await page.getByRole('button', { name: /send through gateway/i }).click();
  await page.waitForTimeout(4000);

  // Expand the promoted corpus so the region is visible.
  const corpusBtn = page.getByRole('button', { name: /promoted corpus/i });
  if (await corpusBtn.count()) { await corpusBtn.first().click(); await page.waitForTimeout(500); }

  // Compact the per-request table to misses only.
  const onlyMisses = page.getByText(/only misses/i);
  if (await onlyMisses.count()) { await onlyMisses.first().click(); await page.waitForTimeout(500); }

  await page.waitForTimeout(1000);
  await page.screenshot({ path: path.join(OUT, 'gateway-harness-readout.png'), fullPage: true });

  // Annotate: outline each region and pin a numbered badge at its top-left.
  const callouts = [
    { n: 1, find: { role: 'button', text: /^(run traffic|running…|starting…)$/i }, label: 'Run Traffic' },
    { n: 2, find: { text: /pattern pack v\d+/i, chip: true }, label: 'Status strip' },
    { n: 3, find: { heading: /^try a prompt$/i }, label: 'Try a prompt' },
    { n: 4, find: { heading: /^run configuration$/i }, label: 'Run configuration' },
    { n: 5, find: { heading: /^(last run|run in progress)/i }, label: 'Last run' },
    { n: 6, find: { heading: /^score$/i }, label: 'Score' },
    { n: 7, find: { heading: /^recall ratchet$/i }, label: 'Recall ratchet' },
    { n: 8, find: { heading: /^flywheel/i }, label: 'Proposals' },
    { n: 9, find: { heading: /^per-request results$/i }, label: 'Per-request results' },
    { n: 10, find: { heading: /^promoted corpus/i }, label: 'Promoted corpus' },
  ];

  const placed = await page.evaluate((callouts) => {
    const out = [];
    const all = Array.from(document.querySelectorAll('h1,h2,h3,h4,div,span,button,summary'));
    function panelFor(el) {
      // Walk up to the nearest block that spans most of the content column.
      let cur = el, best = el;
      const target = Math.min(document.body.clientWidth * 0.5, 700);
      for (let i = 0; i < 8 && cur && cur !== document.body; i++) {
        cur = cur.parentElement;
        if (!cur) break;
        const r = cur.getBoundingClientRect();
        if (r.width >= target && r.height < window.innerHeight * 3) { best = cur; break; }
      }
      return best;
    }
    for (const c of callouts) {
      let el = null;
      if (c.find.role === 'button') {
        el = Array.from(document.querySelectorAll('button')).find(b => new RegExp(c.find.text.source ?? c.find.text, 'i').test(b.textContent.trim()));
      } else if (c.find.chip) {
        const re = new RegExp(c.find.text.source ?? c.find.text, 'i');
        const hits = all.filter(e => re.test(e.textContent.trim()) && e.getBoundingClientRect().width < 420);
        hits.sort((a, b) => {
          const ra = a.getBoundingClientRect(), rb = b.getBoundingClientRect();
          return ra.width * ra.height - rb.width * rb.height;
        });
        el = hits[0] ? hits[0].parentElement : null;
      } else if (c.find.heading) {
        const re = new RegExp(c.find.heading.source ?? c.find.heading, 'i');
        el = all.find(e => e.children.length === 0 && re.test(e.textContent.trim()));
      } else if (c.find.text) {
        const re = new RegExp(c.find.text.source ?? c.find.text, 'i');
        el = all.find(e => e.children.length === 0 && re.test(e.textContent.trim()));
      }
      if (!el) { out.push({ n: c.n, ok: false, label: c.label }); continue; }
      const panel = c.find.role === 'button' || c.find.chip ? el : panelFor(el);
      panel.style.outline = '3px solid #0891B2';
      panel.style.outlineOffset = '3px';
      panel.style.borderRadius = panel.style.borderRadius || '8px';
      const r = panel.getBoundingClientRect();
      const badge = document.createElement('div');
      badge.textContent = String(c.n);
      Object.assign(badge.style, {
        position: 'absolute',
        left: Math.max(4, r.left + window.scrollX - 46) + 'px',
        top: (r.top + window.scrollY - 8) + 'px',
        width: '34px', height: '34px', borderRadius: '50%',
        background: '#0891B2', color: '#fff',
        font: '700 19px/34px system-ui, sans-serif', textAlign: 'center',
        boxShadow: '0 2px 6px rgba(0,0,0,.45)', zIndex: 99999,
      });
      document.body.appendChild(badge);
      out.push({ n: c.n, ok: true, label: c.label });
    }
    return out;
  }, callouts.map(c => ({ n: c.n, label: c.label, find: { role: c.find.role, chip: c.find.chip, heading: c.find.heading ? c.find.heading.source : undefined, text: c.find.text ? c.find.text.source : undefined } })));

  console.log(JSON.stringify(placed, null, 2));
  await page.waitForTimeout(400);
  await page.screenshot({ path: path.join(OUT, 'gateway-harness-readout-annotated.png'), fullPage: true });
  await browser.close();
})().catch(e => { console.error(e); process.exit(1); });
