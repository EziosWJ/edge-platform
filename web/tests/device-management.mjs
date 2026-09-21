import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const port = 4177;
const baseUrl = `http://127.0.0.1:${port}`;
const vite = spawn(
  process.execPath,
  ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1", "--port", String(port)],
  {
    cwd: new URL("..", import.meta.url),
    stdio: ["ignore", "pipe", "pipe"],
  },
);

async function waitForServer() {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try {
      const response = await fetch(`${baseUrl}/tests/device-management.html`);
      if (response.ok) return;
    } catch {
      // Vite is still starting.
    }
    await delay(250);
  }
  throw new Error("Vite test server did not start");
}

async function waitFor(predicate, message) {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    if (await predicate()) return;
    await delay(100);
  }
  throw new Error(message);
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
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    await page.goto(`${baseUrl}/tests/device-management.html`, {
      waitUntil: "networkidle",
    });
    await page.getByText("cloud-device-initial", { exact: true }).waitFor();
    await page.getByRole("cell", { name: "初始", exact: true }).waitFor();
    await page.getByRole("cell", { name: "降级", exact: true }).waitFor();
    await page.getByRole("cell", { name: "离线", exact: true }).waitFor();

    const initialRequest = await page.evaluate(() => window.deviceRequests[0]);
    const initialUrl = new URL(initialRequest, baseUrl);
    if (initialUrl.searchParams.get("edgeId") !== "edge-a") {
      throw new Error(`Edge filter was not carried into Device query: ${initialRequest}`);
    }

    await page.getByLabel("来源 Device ID").fill("source-03");
    await page.getByLabel("筛选通信状态").selectOption("DEGRADED");
    await page.getByRole("button", { name: "查询" }).click();
    await waitFor(
      async () =>
        (await page.evaluate(() => window.deviceRequests)).some((request) =>
          request.includes("sourceDeviceId=source-03"),
        ),
      "filtered Device query was not sent",
    );
    const filteredRequest = await page.evaluate(() =>
      window.deviceRequests.find((request) => request.includes("sourceDeviceId=source-03")),
    );
    const filteredUrl = new URL(filteredRequest, baseUrl);
    if (
      filteredUrl.searchParams.get("status") !== "DEGRADED" ||
      filteredUrl.searchParams.get("sourceDeviceId") !== "source-03"
    ) {
      throw new Error(`unexpected filtered query: ${filteredRequest}`);
    }

    await page.getByRole("button", { name: "查看 cloud-device-degraded 详情" }).click();
    await page.getByText("Device 详情", { exact: true }).waitFor();
    await page.getByRole("dialog").getByText("当前通信诊断", { exact: true }).waitFor();
    if (
      !(await page.evaluate(() => window.deviceRequests)).includes(
        "/api/device/cloud-device-degraded",
      )
    ) {
      throw new Error("Device detail request was not sent");
    }

    await page.getByRole("button", { name: "关闭详情弹窗" }).click();
    await page.getByLabel("每页条数").selectOption("20");
    await waitFor(
      async () =>
        (await page.evaluate(() => window.deviceRequests)).some((request) =>
          request.includes("pageSize=20"),
        ),
      "Device page size query was not sent",
    );
    await page.getByRole("button", { name: "下一页" }).click();
    await waitFor(
      async () =>
        (await page.evaluate(() => window.deviceRequests)).some((request) =>
          request.includes("page=2&pageSize=20"),
        ),
      "Device page query was not sent",
    );

    console.log("device management behavior passed");
  } finally {
    await browser.close();
  }
} finally {
  vite.kill("SIGTERM");
}
