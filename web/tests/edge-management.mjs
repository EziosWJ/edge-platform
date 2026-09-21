import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const port = 4176;
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
      const response = await fetch(`${baseUrl}/tests/edge-management.html`);
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
    const page = await browser.newPage({ viewport: { width: 1100, height: 800 } });
    await page.goto(`${baseUrl}/tests/edge-management.html`, {
      waitUntil: "networkidle",
    });
    await page.getByText("edge-offline", { exact: true }).waitFor();

    const deviceLink = page.getByRole("link", {
      name: "查看 edge-offline 下的设备",
    });
    if ((await deviceLink.getAttribute("href")) !== "/device?edgeId=edge-offline") {
      throw new Error("Edge to Device navigation link was not scoped to the Edge");
    }

    await page.getByLabel("Edge ID").fill("edge-offline");
    await page.getByLabel("筛选状态").selectOption("OFFLINE");
    await page.getByRole("button", { name: "查询" }).click();
    await page.getByText("edge-offline", { exact: true }).waitFor();

    const filteredRequest = await page.evaluate(() =>
      window.edgeRequests.find((request) => request.includes("edgeId=edge-offline")),
    );
    if (!filteredRequest) throw new Error("filtered Edge query was not sent");
    const filteredUrl = new URL(filteredRequest, baseUrl);
    if (
      filteredUrl.searchParams.get("status") !== "OFFLINE" ||
      filteredUrl.searchParams.get("edgeId") !== "edge-offline" ||
      filteredUrl.searchParams.get("page") !== "1" ||
      filteredUrl.searchParams.get("pageSize") !== "10"
    ) {
      throw new Error(`unexpected filtered query: ${filteredRequest}`);
    }

    await page.getByRole("button", { name: "查看 edge-offline 详情" }).click();
    await page.getByText("Edge 详情", { exact: true }).waitFor();
    await page
      .getByRole("dialog")
      .getByText("首次登记时间（UTC）", { exact: true })
      .waitFor();
    const detailRequestSeen = await page.evaluate(() =>
      window.edgeRequests.includes("/api/edge/edge-offline"),
    );
    if (!detailRequestSeen) throw new Error("Edge detail request was not sent");

    await page.getByRole("button", { name: "关闭详情弹窗" }).click();
    await page.getByLabel("每页条数").selectOption("20");
    await waitFor(
      async () =>
        (await page.evaluate(() => window.edgeRequests)).some((request) =>
          request.includes("pageSize=20"),
        ),
      "page size query was not sent",
    );
    await page.getByRole("button", { name: "下一页" }).click();
    await waitFor(
      async () =>
        (await page.evaluate(() => window.edgeRequests)).some((request) =>
          request.includes("page=2&pageSize=20"),
        ),
      "page query was not sent",
    );

    console.log("edge management behavior passed");
  } finally {
    await browser.close();
  }
} finally {
  vite.kill("SIGTERM");
}
