import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const port = 4174;
const baseUrl = `http://127.0.0.1:${port}`;
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", String(port)], {
  cwd: new URL("..", import.meta.url),
  stdio: ["ignore", "pipe", "pipe"],
});

async function waitForServer() {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try {
      const response = await fetch(`${baseUrl}/tests/operation-column-layout.html`);
      if (response.ok) return;
    } catch {
      // Vite is still starting.
    }
    await delay(250);
  }
  throw new Error("Vite test server did not start");
}

try {
  await waitForServer();
  const browser = await chromium.launch({
    headless: true,
    ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH
      ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH }
      : {}),
  });
  try {
    const page = await browser.newPage({ viewport: { width: 320, height: 480 } });
    await page.goto(`${baseUrl}/tests/operation-column-layout.html`, { waitUntil: "networkidle" });
    await page.getByTestId("single-action-label").waitFor();

    const result = await page.evaluate(() => {
      const label = document.querySelector('[data-testid="single-action-label"]');
      const button = label?.closest("button");
      const cell = button?.closest("td");
      const header = cell?.closest("table")?.querySelector("thead th:last-child");
      if (!label || !button || !cell) throw new Error("operation fixture did not render");
      return {
        labelLines: label.getClientRects().length,
        buttonWhiteSpace: getComputedStyle(button).whiteSpace,
        buttonHeight: button.getBoundingClientRect().height,
        buttonWidth: button.getBoundingClientRect().width,
        cellWidth: cell.getBoundingClientRect().width,
        headerWidth: header?.getBoundingClientRect().width ?? 0,
      };
    });

    if (result.labelLines !== 1) throw new Error(`single action label wrapped: ${JSON.stringify(result)}`);
    if (result.buttonWhiteSpace !== "nowrap") throw new Error(`button allows wrapping: ${JSON.stringify(result)}`);
    if (result.buttonHeight > 32) throw new Error(`single action button grew vertically: ${JSON.stringify(result)}`);
    if (result.cellWidth < result.buttonWidth) throw new Error(`operation cell clips its button: ${JSON.stringify(result)}`);
    if (Math.abs(result.cellWidth - result.headerWidth) > 1) throw new Error(`header and body widths diverged: ${JSON.stringify(result)}`);

    const multiActionLines = await page.getByTestId("multi-action-label").evaluate((element) => element.getClientRects().length);
    if (multiActionLines !== 1) throw new Error("multi-action button label wrapped internally");
    const multiActionHeights = await page.locator("td button").evaluateAll((buttons) => buttons.map((button) => button.getBoundingClientRect().height));
    if (multiActionHeights.some((height) => height > 32)) throw new Error(`multi-action button grew vertically: ${JSON.stringify(multiActionHeights)}`);
    console.log("operation-column layout passed", JSON.stringify(result));
  } finally {
    await browser.close();
  }
} finally {
  vite.kill("SIGTERM");
}
