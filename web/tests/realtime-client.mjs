import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const port = 4178;
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
      const response = await fetch(`${baseUrl}/tests/realtime-client.html`);
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
    await page.goto(`${baseUrl}/tests/realtime-client.html`, { waitUntil: "networkidle" });
    await page.getByText("temperature", { exact: true }).waitFor();
    await page.getByRole("button", { name: "详情" }).first().click();
    await page.getByText("柜内温度 · temperature", { exact: true }).waitFor();
    await page.getByText("实时已连接", { exact: true }).waitFor();

    const connection = await page.evaluate(() => {
      const ticketRequest = window.realtimeRequests.find(
        (request) => request.path === "/api/realtime/ticket",
      );
      const socket = window.realtimeTest.socket;
      if (!ticketRequest || !socket) throw new Error("realtime connection was not created");
      return {
        ticketMethod: ticketRequest.method,
        url: socket.url,
        subscribe: JSON.parse(socket.sent[0]),
      };
    });
    const socketUrl = new URL(connection.url);
    if (connection.ticketMethod !== "POST") throw new Error("ticket was not requested with POST");
    if (socketUrl.searchParams.get("ticket") !== "short-lived-ticket") throw new Error("ticket was not passed to WebSocket");
    if ([...socketUrl.searchParams.keys()].some((key) => key.toLowerCase().includes("token"))) {
      throw new Error(`long-lived token appeared in WebSocket URL: ${connection.url}`);
    }
    if (
      connection.subscribe.type !== "subscribe" ||
      connection.subscribe.points?.length !== 2 ||
      connection.subscribe.points[0].deviceId !== "device-realtime" ||
      connection.subscribe.points[0].pointKey !== "temperature"
    ) {
      throw new Error(`unexpected batch subscription: ${JSON.stringify(connection.subscribe)}`);
    }
    if (connection.subscribe.points[1].pointKey !== "pump_enabled") {
      throw new Error(`boolean point was not subscribed: ${JSON.stringify(connection.subscribe)}`);
    }

    if (socketUrl.searchParams.size !== 1 || socketUrl.searchParams.has("jwt")) {
      throw new Error(`WebSocket URL carried unexpected credentials: ${connection.url}`);
    }

    // The live update can win the race with the snapshot. The client/store
    // must keep the higher persisted revision regardless of arrival order.
    await page.evaluate(() => {
      window.realtimeTest.socket.emit({
        type: "update",
        dataPointId: "point-temperature",
        deviceId: "device-realtime",
        pointKey: "temperature",
        valueType: "NUMBER",
        value: 13.4,
        quality: "GOOD",
        sourceTimestamp: "2026-09-22T00:02:00Z",
        observedAt: "2026-09-22T00:02:01Z",
        revision: 3,
      });
    });

    await page.evaluate(() => {
      window.realtimeTest.socket.emit({
        type: "snapshot",
        dataPointId: "point-temperature",
        deviceId: "device-realtime",
        pointKey: "temperature",
        valueType: "NUMBER",
        value: 12.3,
        quality: "GOOD",
        sourceTimestamp: "2026-09-22T00:01:00Z",
        observedAt: "2026-09-22T00:01:01Z",
        revision: 2,
      });
    });
    await page.getByText("当前值：13.4 °C", { exact: true }).waitFor();
    await page.getByText("revision：3", { exact: true }).waitFor();

    await page.evaluate(() => {
      window.realtimeTest.socket.emit({
        type: "update",
        dataPointId: "point-temperature",
        deviceId: "device-realtime",
        pointKey: "temperature",
        valueType: "NUMBER",
        value: 99.9,
        quality: "GOOD",
        sourceTimestamp: "2026-09-22T00:03:00Z",
        observedAt: "2026-09-22T00:03:01Z",
        revision: 3,
      });
    });
    await page.getByText("当前值：13.4 °C", { exact: true }).waitFor();
    await page.getByText("revision：3", { exact: true }).waitFor();

    await page.evaluate(() => {
      window.realtimeTest.socket.emit({
        type: "update",
        dataPointId: "point-temperature",
        deviceId: "device-realtime",
        pointKey: "temperature",
        valueType: "NUMBER",
        value: 1.1,
        quality: "GOOD",
        sourceTimestamp: "2026-09-22T00:00:10Z",
        observedAt: "2026-09-22T00:00:11Z",
        revision: 1,
      });
    });
    await delay(100);
    const lowerRevisionText = await page.getByRole("dialog").innerText();
    if (
      lowerRevisionText.includes("1.1 °C") ||
      !lowerRevisionText.includes("当前值：13.4 °C") ||
      !lowerRevisionText.includes("revision：3")
    ) {
      throw new Error(`lower revision was not ignored: ${lowerRevisionText}`);
    }

    await page.evaluate(() => {
      window.realtimeTest.socket.emit({
        type: "update",
        dataPointId: "point-temperature",
        deviceId: "device-realtime",
        pointKey: "temperature",
        valueType: "NUMBER",
        value: 14.5,
        quality: "GOOD",
        sourceTimestamp: "2026-09-22T00:04:00Z",
        observedAt: "2026-09-22T00:04:01Z",
        revision: 4,
      });
    });
    await page.getByText("当前值：14.5 °C", { exact: true }).waitFor();
    await page.getByText("质量：GOOD", { exact: true }).waitFor();
    await page.getByText("revision：4", { exact: true }).waitFor();

    await page.evaluate(() => {
      window.realtimeTest.socket.emit({
        type: "update",
        dataPointId: "point-temperature",
        deviceId: "device-realtime",
        pointKey: "temperature",
        valueType: "NUMBER",
        value: 14.5,
        quality: "BAD",
        sourceTimestamp: "2026-09-22T00:04:00Z",
        observedAt: "2026-09-22T00:05:01Z",
        revision: 5,
      });
    });
    await page.getByText("当前值：14.5 °C", { exact: true }).waitFor();
    await page.getByText("质量：BAD", { exact: true }).waitFor();
    await page.getByText("revision：5", { exact: true }).waitFor();

    await page.evaluate(() => {
      window.realtimeTest.socket.emit({
        type: "update",
        dataPointId: "point-temperature",
        deviceId: "device-realtime",
        pointKey: "temperature",
        valueType: "NUMBER",
        value: null,
        quality: "NO_DATA",
        sourceTimestamp: null,
        observedAt: null,
        revision: 6,
      });
    });
    await page.getByText("当前值：—", { exact: true }).waitFor();
    await page.getByText("质量：NO_DATA", { exact: true }).waitFor();
    await page.getByText("sourceTimestamp：—", { exact: true }).waitFor();
    await page.getByText("observedAt：—", { exact: true }).waitFor();
    await page.getByText("revision：6", { exact: true }).waitFor();

    await page.evaluate(() => {
      window.realtimeTest.socket.emit({
        type: "update",
        dataPointId: "point-enabled",
        deviceId: "device-realtime",
        pointKey: "pump_enabled",
        valueType: "BOOLEAN",
        value: true,
        quality: "GOOD",
        sourceTimestamp: "2026-09-22T00:06:00Z",
        observedAt: "2026-09-22T00:06:01Z",
        revision: 2,
      });
    });
    await page.getByText("true", { exact: true }).waitFor();

    const socketsBeforeRestart = await page.evaluate(() => window.realtimeTest.sockets.length);
    await page.evaluate(() => window.realtimeTest.socket?.serverClose());
    await waitFor(
      () => page.evaluate((before) => window.realtimeTest.sockets.length > before, socketsBeforeRestart),
      "realtime client did not reconnect after server close",
    );
    const recovery = await page.evaluate(() => {
      const ticketRequests = window.realtimeRequests.filter((request) => request.path === "/api/realtime/ticket");
      const socket = window.realtimeTest.socket;
      return { ticketCount: ticketRequests.length, subscribe: socket ? JSON.parse(socket.sent[0]) : null };
    });
    if (recovery.ticketCount < 2 || recovery.subscribe?.type !== "subscribe" || recovery.subscribe.points?.length !== 2) {
      throw new Error(`reconnect did not get a new ticket and batch subscription: ${JSON.stringify(recovery)}`);
    }

    await page.getByRole("button", { name: "关闭详情弹窗" }).click();
    await page.getByLabel("pointKey").fill("pump_enabled");
    await page.getByRole("button", { name: "查询" }).click();
    await page.getByText("pump_enabled", { exact: true }).waitFor();
    const unsubscribe = await page.evaluate(() => {
      const messages = window.realtimeTest.socket?.sent.map((value) => JSON.parse(value)) ?? [];
      return messages.find((message) => message.type === "unsubscribe");
    });
    if (
      unsubscribe?.points?.length !== 1 ||
      unsubscribe.points[0].deviceId !== "device-realtime" ||
      unsubscribe.points[0].pointKey !== "temperature"
    ) {
      throw new Error(`filter did not unsubscribe hidden point: ${JSON.stringify(unsubscribe)}`);
    }
    console.log("realtime batch-list behavior passed");
  } finally {
    await browser.close();
  }
} finally {
  vite.kill("SIGTERM");
}
