import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const port = 4175;
const baseUrl = `http://127.0.0.1:${port}`;
const vite = spawn(process.execPath, ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", String(port)], {
  cwd: new URL("..", import.meta.url),
  stdio: ["ignore", "pipe", "pipe"],
});

async function waitForServer() {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try {
      const response = await fetch(`${baseUrl}/tests/route-loading.html`);
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
    const page = await browser.newPage({ viewport: { width: 640, height: 480 } });
    await page.goto(`${baseUrl}/tests/route-loading.html`, { waitUntil: "networkidle" });
    await page.evaluate(() => {
      window.routeLoadingSeen = false;
      new MutationObserver(() => {
        if (document.querySelector('[role="status"][aria-label="正在切换页面"]')) window.routeLoadingSeen = true;
      }).observe(document.body, { childList: true, subtree: true });
    });
    await page.getByRole("link", { name: "打开即时页面" }).click();
    await page.getByText("即时页", { exact: true }).waitFor();
    if (!await page.evaluate(() => window.routeLoadingSeen)) throw new Error("route loading was not rendered for an immediate navigation");

    await page.goto(`${baseUrl}/tests/route-loading.html`, { waitUntil: "networkidle" });
    await page.getByRole("link", { name: "打开报表" }).click();
    await page.getByRole("status", { name: "正在切换页面" }).waitFor();
    await page.getByText("报表页").waitFor();
    await page.getByRole("status", { name: "正在切换页面" }).waitFor({ state: "detached" });
    console.log("route loading passed");
  } finally {
    await browser.close();
  }
} finally {
  vite.kill("SIGTERM");
}
