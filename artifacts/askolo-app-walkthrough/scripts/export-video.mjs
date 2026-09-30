// Captures the same browser animation shown in the artifact preview, then
// builds a constant-frame-rate video from timestamped screencast frames.
// Run from the workspace root while the managed web workflow is running.
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { chromium } from '../../../node_modules/.pnpm/playwright-core@1.63.0/node_modules/playwright-core/index.mjs';

const outputDir = resolve('artifacts/askolo-app-walkthrough/exports');
const frameDir = resolve(outputDir, 'frames');
await mkdir(frameDir, { recursive: true });
const browser = await chromium.launch({
  executablePath: '/repl/tools/bin/chromium',
  headless: true,
  args: ['--no-sandbox', '--disable-background-timer-throttling', '--disable-renderer-backgrounding'],
});

const context = await browser.newContext({
  viewport: { width: 1280, height: 720 },
  deviceScaleFactor: 1,
  reducedMotion: 'no-preference',
});
const page = await context.newPage();
const cdp = await context.newCDPSession(page);
await cdp.send('Page.enable');
const frames = [];
const pending = [];
let startTime = 0;
let capture = false;

cdp.on('Page.screencastFrame', event => {
  const now = performance.now();
  const promise = (async () => {
    if (capture) {
      const number = frames.length;
      const file = resolve(frameDir, `${String(number).padStart(5, '0')}.jpg`);
      frames.push({ file, t: now - startTime });
      await writeFile(file, Buffer.from(event.data, 'base64'));
    }
    await cdp.send('Page.screencastFrameAck', { sessionId: event.sessionId });
  })();
  pending.push(promise);
});

try {
  await cdp.send('Page.startScreencast', { format: 'jpeg', quality: 87, maxWidth: 1280, maxHeight: 720, everyNthFrame: 1 });
  await page.goto('http://127.0.0.1:80/askolo-app-walkthrough/', { waitUntil: 'domcontentloaded' });
  await page.locator('.shot img').first().waitFor({ state: 'visible' });
  await page.waitForFunction(() => [...document.querySelectorAll('.shot img')].every(img => img.complete));
  startTime = performance.now();
  capture = true;
  await new Promise(resolve => setTimeout(resolve, 44000));
  capture = false;
  await cdp.send('Page.stopScreencast');
  await Promise.all(pending);
  if (frames.length < 130) throw new Error(`Only ${frames.length} frames captured`);
  const list = frames.map((frame, i) => {
    const next = frames[i + 1]?.t ?? 44000;
    return `file '${frame.file}'\nduration ${Math.max(0.02, (next - frame.t) / 1000).toFixed(5)}\n`;
  }).join('') + `file '${frames.at(-1).file}'\n`;
  await writeFile(resolve(outputDir, 'frames.txt'), list);
  await writeFile(resolve(outputDir, 'capture-stats.json'), JSON.stringify({
    count: frames.length, firstMs: frames[0].t, lastMs: frames.at(-1).t,
  }, null, 2));
  console.log(`Captured ${frames.length} frames over ${(frames.at(-1).t / 1000).toFixed(1)} seconds.`);
} finally {
  await browser.close();
}