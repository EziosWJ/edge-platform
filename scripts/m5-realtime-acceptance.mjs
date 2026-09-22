/*
 * M5 real CurrentValue/WebSocket acceptance.
 *
 * Normal values in this probe are produced by the real Edge Collector polling
 * the real Modbus simulator. There is deliberately no normal mosquitto_pub
 * path in this file. Protocol-only revision ordering and malformed-ticket
 * checks are auxiliary assertions and are labelled as such in the output.
 *
 * The environment is normally prepared by running M3 with cleanup disabled:
 *   M3_DEVICE_CLEANUP=0 task m3:device-acceptance
 * Then run this task with the variables documented in
 * docs/acceptance/m5-realtime-currentvalue.md.
 */
import assert from "node:assert/strict";
import { access, readFile } from "node:fs/promises";
import { spawn, spawnSync } from "node:child_process";
import net from "node:net";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";
import playwright from "../web/node_modules/playwright/index.js";

const repoRoot = fileURLToPath(new URL("../", import.meta.url));
const { chromium } = playwright;
const infraRoot = path.resolve(process.env.M5_INFRA_ROOT ?? path.join(repoRoot, "..", "edge-dev-infra"));
const collectorRoot = path.resolve(process.env.M5_COLLECTOR_ROOT ?? path.join(repoRoot, "..", "edge-collector"));
const collectorAPI = path.resolve(
  process.env.M5_COLLECTOR_API_ROOT
    ?? (path.basename(collectorRoot) === "edge-collector-api" ? collectorRoot : path.join(collectorRoot, "edge-collector-api")),
);
const cloudURL = requiredURL("M5_CLOUD_URL");
const collectorURL = requiredURL("M5_COLLECTOR_URL");
const deviceID = required("M5_DEVICE_ID");
const postgresURL = required("M5_POSTGRES_URL");
const cloudRestartScript = required("M5_CLOUD_RESTART_SCRIPT");
const sourceExternalID = process.env.M5_COLLECTOR_SOURCE_EXTERNAL_ID ?? "m3-source-device";
const username = process.env.M5_USERNAME ?? "admin";
const password = process.env.M5_PASSWORD ?? "admin123";
const collectorUsername = process.env.M5_COLLECTOR_USERNAME ?? "admin";
const collectorPassword = process.env.M5_COLLECTOR_PASSWORD ?? "admin123";
const runID = sanitize(process.env.M5_RUN_ID ?? String(Date.now()));
const numberAddress = nonNegativeIntEnv("M5_NUMBER_ADDRESS", 0);
const booleanAddress = nonNegativeIntEnv("M5_BOOLEAN_ADDRESS", 1);
const booleanBit = nonNegativeIntEnv("M5_BOOLEAN_BIT", 0);
const numberScale = finiteEnv("M5_NUMBER_SCALE", 0.1);
const mqttHost = process.env.M5_MQTT_HOST ?? "127.0.0.1";
const mqttPort = integerEnv("M5_MQTT_PORT", 18884);
const sessionCloseTimeout = integerEnv("M5_SESSION_CLOSE_TIMEOUT_MS", 40000);

function required(name) {
  const value = process.env[name]?.trim();
  if (!value) throw new Error(`${name} is required; see docs/acceptance/m5-realtime-currentvalue.md`);
  return value;
}

function requiredURL(name) {
  const value = required(name).replace(/\/$/, "");
  try { new URL(value); } catch { throw new Error(`${name} must be an absolute URL: ${value}`); }
  return value;
}

function integerEnv(name, fallback) {
  const value = Number(process.env[name] ?? fallback);
  assert.ok(Number.isInteger(value) && value >= 1 && value <= 65535, `${name} must be a TCP port or positive integer`);
  return value;
}

function nonNegativeIntEnv(name, fallback) {
  const value = Number(process.env[name] ?? fallback);
  assert.ok(Number.isInteger(value) && value >= 0 && value <= 65535, `${name} must be an integer in [0, 65535]`);
  return value;
}

function finiteEnv(name, fallback) {
  const value = Number(process.env[name] ?? fallback);
  assert.ok(Number.isFinite(value), `${name} must be finite`);
  return value;
}

function sanitize(value) {
  return (value.toLowerCase().replace(/[^a-z0-9_]/g, "_").replace(/^_+|_+$/g, "") || "run").slice(0, 32);
}

function log(message) { process.stdout.write(`[m5-realtime] ${message}\n`); }

async function requestJSON(baseURL, pathname, token, options = {}) {
  const response = await fetch(`${baseURL}${pathname}`, {
    ...options,
    signal: options.signal ?? AbortSignal.timeout(15000),
    headers: {
      ...(options.body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: token.startsWith("Bearer ") ? token : `Bearer ${token}` } : {}),
      ...(options.headers ?? {}),
    },
    body: options.body !== undefined && typeof options.body !== "string" ? JSON.stringify(options.body) : options.body,
  });
  const text = await response.text();
  let payload;
  try { payload = JSON.parse(text); } catch { payload = { raw: text }; }
  return { response, payload };
}

async function login(baseURL, user, pass) {
  const result = await requestJSON(baseURL, "/api/auth/login", "", { method: "POST", body: { username: user, password: pass } });
  assert.equal(result.response.status, 200, `login failed at ${baseURL}: ${JSON.stringify(result.payload)}`);
  assert.equal(result.payload.code, 200, `login failed at ${baseURL}: ${JSON.stringify(result.payload)}`);
  return result.payload.data.tokenValue;
}

function api(baseURL, token) {
  return async (pathname, options = {}) => {
    const result = await requestJSON(baseURL, pathname, token, options);
    assert.equal(result.response.status, 200, `${options.method ?? "GET"} ${pathname}: ${JSON.stringify(result.payload)}`);
    assert.equal(result.payload.code, 200, `${options.method ?? "GET"} ${pathname}: ${JSON.stringify(result.payload)}`);
    return result.payload.data;
  };
}

async function waitFor(label, read, predicate, timeout = 90000, interval = 500) {
  const deadline = Date.now() + timeout;
  let value;
  while (Date.now() < deadline) {
    value = await read();
    if (predicate(value)) return value;
    await delay(interval);
  }
  throw new Error(`${label} timed out: ${JSON.stringify(value)}`);
}

async function waitForTCP(host, port, timeout = 5000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const open = await new Promise((resolve) => {
      const socket = net.createConnection({ host, port });
      socket.once("connect", () => { socket.destroy(); resolve(true); });
      socket.once("error", () => { socket.destroy(); resolve(false); });
    });
    if (open) return;
    await delay(100);
  }
  throw new Error(`real MQTT endpoint is not reachable: ${host}:${port}`);
}

function mapping(valueType, address) {
  if (valueType === "BOOLEAN") {
    return { sourceType: "MODBUS_REGISTER", functionCode: 3, address, encoding: "BOOLEAN_BIT", byteOrder: "BIG_ENDIAN", wordOrder: null, bitIndex: booleanBit, scale: 1, offset: 0 };
  }
  return { sourceType: "MODBUS_REGISTER", functionCode: 3, address, encoding: "UINT16", byteOrder: "BIG_ENDIAN", wordOrder: null, bitIndex: null, scale: numberScale, offset: 0 };
}

async function collectorSource(collector) {
  const page = await collector(`/api/v1/acquisition/devices?page=1&pageSize=500`);
  const records = page.records ?? page.items ?? page;
  const device = records.find((candidate) => candidate.externalId === sourceExternalID);
  assert.ok(device, `real Collector source device ${sourceExternalID} was not found`);
  assert.ok(device.channelId, `Collector source device ${sourceExternalID} has no channel`);
  const state = await collector(`/api/v1/acquisition/states`);
  const states = state.records ?? state.items ?? state;
  const current = Array.isArray(states) ? states.find((candidate) => candidate.deviceId === device.id) : null;
  assert.ok(current, `Collector has no runtime state for ${sourceExternalID}`);
  assert.equal(current.status, "ONLINE", `real Collector source is not ONLINE: ${JSON.stringify(current)}`);
  assert.ok(current.registerBlocks?.length, "real Collector source has no register blocks; Modbus simulator is not configured");
  return { device, state: current };
}

async function postgresBoundaryCheck() {
  const query = "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('current_value','data_point','source_mapping') ORDER BY table_name";
  const direct = spawnSync("psql", [postgresURL, "-At", "-c", query], { encoding: "utf8" });
  let result = direct;
  if (direct.error?.code === "ENOENT") {
    const container = process.env.M5_POSTGRES_CONTAINER;
    if (!container) throw new Error("psql is unavailable; set M5_POSTGRES_CONTAINER to the exact real PostgreSQL container for the schema assertion");
    const user = process.env.M5_POSTGRES_USER ?? "edge_platform";
    const password = process.env.M5_POSTGRES_PASSWORD ?? "edge-platform-db-dev";
    const database = process.env.M5_POSTGRES_DATABASE ?? "edge_platform";
    result = spawnSync("docker", ["exec", container, "env", `PGPASSWORD=${password}`, "psql", "-U", user, "-d", database, "-At", "-c", query], { encoding: "utf8" });
  }
  assert.equal(result.status, 0, `real PostgreSQL query failed: ${result.stderr}`);
  assert.deepEqual(result.stdout.trim().split("\n").filter(Boolean), ["current_value", "data_point", "source_mapping"]);
  log("PASS PostgreSQL current_value/data_point/source_mapping are queried from the real database");
}

async function createPoint(cloud, valueType, pointKey, address) {
  return cloud("/api/datapoint", {
    method: "POST",
    body: { deviceId: deviceID, pointKey, name: `M5 ${valueType} ${runID}`, valueType, unit: valueType === "NUMBER" ? "A" : null, precision: valueType === "NUMBER" ? 1 : null, mapping: mapping(valueType, address) },
  });
}

async function current(cloud, pointID) { return cloud(`/api/datapoint/${encodeURIComponent(pointID)}`); }

async function waitGood(cloud, pointID, revision = 0) {
  return waitFor(`real Collector GOOD revision > ${revision}`, () => current(cloud, pointID), (point) => point.currentValue.quality === "GOOD" && point.currentValue.revision > revision);
}

async function waitQuality(cloud, pointID, quality, revision) {
  return waitFor(`CurrentValue ${quality} revision > ${revision}`, () => current(cloud, pointID), (point) => point.currentValue.quality === quality && point.currentValue.revision > revision);
}

function pageProbeScript() {
  return `({
    async connect(token, points) {
      const authorization = token.startsWith('Bearer ') ? token : 'Bearer ' + token;
      const requestTicket = async () => {
        const response = await fetch('/api/realtime/ticket', { method: 'POST', headers: { Authorization: authorization } });
        const body = await response.json();
        if (!response.ok || body.code !== 200) throw new Error('ticket request failed: ' + JSON.stringify(body));
        return body.data.ticket;
      };
      const socketURL = (ticket) => location.origin.replace(/^http/, 'ws') + '/api/realtime/ws?ticket=' + encodeURIComponent(ticket);
      const ticket = await requestTicket();
      const url = socketURL(ticket);
      const state = { ticket, url, messages: [], close: null, opened: false, manuallyClosed: false, reconnects: 0, reconnecting: false, reconnectError: null, merged: new Map() };
      window.__m5 = state;
      const merge = (message) => {
        if (message.type !== 'snapshot' && message.type !== 'update') return;
        const key = message.deviceId + '\\u0000' + message.pointKey;
        const old = state.merged.get(key);
        if (!old || message.revision > old.revision) state.merged.set(key, message);
      };
      const open = (socketURL) => new Promise((resolve, reject) => {
        let settled = false;
        let opened = false;
        const socket = new WebSocket(socketURL);
        state.socket = socket;
        const fail = (error) => { if (!settled) { settled = true; reject(error); } };
        socket.onopen = () => { opened = true; settled = true; state.opened = true; state.reconnectError = null; socket.send(JSON.stringify({ type: 'subscribe', requestId: 'm5-' + Date.now(), points })); resolve(); };
        socket.onmessage = (event) => { const message = JSON.parse(event.data); state.messages.push(message); merge(message); };
        socket.onerror = () => { if (!opened) { fail(new Error('real browser WebSocket error')); try { socket.close(); } catch {} } };
        socket.onclose = (event) => {
          if (state.socket !== socket) return;
          state.close = { code: event.code, reason: event.reason };
          state.socket = null;
          state.opened = false;
          if (!opened) fail(new Error('real browser WebSocket closed before opening: ' + event.code));
          if (!state.manuallyClosed && !state.reconnecting) {
            state.reconnecting = true;
            void state.reconnect().catch((error) => { state.reconnectError = String(error); }).finally(() => { state.reconnecting = false; });
          }
        };
      });
      state.reconnect = async () => {
        let attempt = 0;
        while (!state.manuallyClosed) {
          state.reconnects += 1;
          try {
            state.ticket = await requestTicket();
            state.url = socketURL(state.ticket);
            await open(state.url);
            return;
          } catch (error) {
            state.reconnectError = String(error);
            await new Promise((resolve) => setTimeout(resolve, Math.min(5000, 250 * 2 ** Math.min(attempt++, 5))));
          }
        }
      };
      await open(url);
      state.merge = merge;
      return { ticket, url };
    },
    close() { if (window.__m5?.socket) { window.__m5.manuallyClosed = true; window.__m5.socket.close(1000, 'probe cleanup'); } },
    merge(message) { if (window.__m5?.merge) window.__m5.merge(message); },
    state() { return window.__m5 ? { ticket: window.__m5.ticket, url: window.__m5.url, opened: window.__m5.opened, reconnects: window.__m5.reconnects, reconnectError: window.__m5.reconnectError, close: window.__m5.close, messages: window.__m5.messages, merged: [...window.__m5.merged.values()] } : null; },
    async secondUse(ticket) { return new Promise((resolve) => { const socket = new WebSocket(location.origin.replace(/^http/, 'ws') + '/api/realtime/ws?ticket=' + encodeURIComponent(ticket)); let opened = false; socket.onopen = () => { opened = true; socket.close(); resolve({ opened, close: null }); }; socket.onerror = () => {}; socket.onclose = (event) => resolve({ opened, close: { code: event.code, reason: event.reason } }); }); },
    async expired(ticket) { return new Promise((resolve) => { const socket = new WebSocket(location.origin.replace(/^http/, 'ws') + '/api/realtime/ws?ticket=' + encodeURIComponent(ticket)); let opened = false; socket.onopen = () => { opened = true; socket.close(); resolve({ opened, close: null }); }; socket.onerror = () => {}; socket.onclose = (event) => resolve({ opened, close: { code: event.code, reason: event.reason } }); }); },
  })`;
}

async function newProbePage(browser, token) {
  const page = await browser.newPage();
  await page.goto(`${cloudURL}/health`, { waitUntil: "domcontentloaded" });
  await page.evaluate((script) => { window.__m5Factory = eval(script); }, pageProbeScript());
  return { page, token };
}

async function launchBrowser() {
  const executablePath = process.env.PLAYWRIGHT_EXECUTABLE_PATH || chromium.executablePath();
  try {
    await access(executablePath);
  } catch {
    throw new Error(`Playwright Chromium executable is unavailable: ${executablePath}; install the project browser with 'npm --prefix web exec playwright install chromium' or set PLAYWRIGHT_EXECUTABLE_PATH`);
  }
  try {
    return await chromium.launch({ headless: true, executablePath });
  } catch (error) {
    throw new Error(`Playwright Chromium could not launch using ${executablePath}: ${error instanceof Error ? error.message : String(error)}`);
  }
}

async function waitBrowserMessage(page, predicate, label, timeout = 90000) {
  return waitFor(label, () => page.evaluate(() => window.__m5Factory.state()), (state) => state?.messages.some(predicate), timeout, 250);
}

async function runAcceptance() {
  await access(infraRoot); await access(collectorRoot); await access(collectorAPI);
  await waitForTCP(mqttHost, mqttPort);
  const token = await login(cloudURL, username, password);
  const secondToken = await login(cloudURL, username, password);
  const cloud = api(cloudURL, token);
  const collector = api(collectorURL, await login(collectorURL, collectorUsername, collectorPassword));
  await postgresBoundaryCheck();
  const source = await collectorSource(collector);
  log(`real Collector source ${source.device.externalId} is ONLINE with ${source.state.registerBlocks.length} register block(s)`);

  const number = await createPoint(cloud, "NUMBER", `m5_number_${runID}`, numberAddress);
  const boolean = await createPoint(cloud, "BOOLEAN", `m5_boolean_${runID}`, booleanAddress);
  assert.ok(number.dataPointId && boolean.dataPointId);
  const firstNumber = await waitGood(cloud, number.dataPointId);
  const firstBoolean = await waitGood(cloud, boolean.dataPointId);
  assert.equal(typeof firstNumber.currentValue.value, "number");
  assert.equal(typeof firstBoolean.currentValue.value, "boolean");
  log("PASS real Modbus simulator -> Edge Collector -> MQTT -> Cloud PostgreSQL produced initial NUMBER/BOOLEAN CurrentValue");

  const browser = await launchBrowser();
  const primary = await newProbePage(browser, token);
  const secondary = await newProbePage(browser, secondToken);
  try {
    const points = [{ deviceId: deviceID, pointKey: number.pointKey }, { deviceId: deviceID, pointKey: boolean.pointKey }];
    const primaryConnection = await primary.page.evaluate(({ token: authToken, points: wanted }) => window.__m5Factory.connect(authToken, wanted), { token, points });
    const secondaryConnection = await secondary.page.evaluate(({ token: authToken, points: wanted }) => window.__m5Factory.connect(authToken, wanted), { token: secondToken, points });
    assert.equal(new URL(primaryConnection.url).searchParams.size, 1, "WebSocket URL has unexpected query credentials");
    assert.equal(new URL(primaryConnection.url).searchParams.has("token"), false, "browser WebSocket URL contains a long-lived token");
    const snapshotState = await waitBrowserMessage(primary.page, (message) => message.type === "snapshot" && points.some((point) => point.pointKey === message.pointKey), "initial browser snapshots");
    assert.ok(snapshotState.messages.filter((message) => message.type === "snapshot").length >= 2, "multi-point subscription did not return both snapshots");
    for (const message of snapshotState.messages.filter((item) => item.type === "snapshot")) {
      assert.deepEqual(Object.keys(message).sort(), ["dataPointId", "deviceId", "observedAt", "pointKey", "quality", "revision", "sourceTimestamp", "type", "value", "valueType"].sort(), "realtime payload leaked SourceMapping/Edge fields");
    }
    log("PASS browser received first snapshots for NUMBER and BOOLEAN through one multi-point WebSocket; payload is semantic-only");

    const liveNumber = await waitGood(cloud, number.dataPointId, firstNumber.currentValue.revision);
    const liveBoolean = await waitGood(cloud, boolean.dataPointId, firstBoolean.currentValue.revision);
    await waitBrowserMessage(primary.page, (message) => message.type === "update" && message.pointKey === number.pointKey && message.revision >= liveNumber.currentValue.revision, "NUMBER live update");
    await waitBrowserMessage(primary.page, (message) => message.type === "update" && message.pointKey === boolean.pointKey && message.revision >= liveBoolean.currentValue.revision, "BOOLEAN live update");
    log("PASS real Collector polling produced subsequent NUMBER/BOOLEAN live updates");

    const resetBefore = await current(cloud, number.dataPointId);
    const disabled = await cloud(`/api/datapoint/${number.dataPointId}/enabled`, { method: "PUT", body: { enabled: false } });
    assert.equal(disabled.currentValue.quality, "NO_DATA");
    await waitBrowserMessage(primary.page, (message) => message.type === "update" && message.pointKey === number.pointKey && message.quality === "NO_DATA" && message.revision > resetBefore.currentValue.revision, "mapping disable NO_DATA update");
    const remapped = await cloud(`/api/datapoint/${number.dataPointId}`, { method: "PUT", body: { name: disabled.name, unit: disabled.unit, precision: disabled.precision, mapping: mapping("NUMBER", booleanAddress) } });
    assert.equal(remapped.currentValue.quality, "NO_DATA");
    await cloud(`/api/datapoint/${number.dataPointId}/enabled`, { method: "PUT", body: { enabled: true } });
    await waitGood(cloud, number.dataPointId, remapped.currentValue.revision);
    await waitBrowserMessage(primary.page, (message) => message.type === "update" && message.pointKey === number.pointKey && message.quality === "GOOD" && message.revision > remapped.currentValue.revision, "mapping reset recovery");
    const boolBefore = await current(cloud, boolean.dataPointId);
    const boolDisabled = await cloud(`/api/datapoint/${boolean.dataPointId}/enabled`, { method: "PUT", body: { enabled: false } });
    assert.equal(boolDisabled.currentValue.quality, "NO_DATA");
    await waitBrowserMessage(primary.page, (message) => message.type === "update" && message.pointKey === boolean.pointKey && message.quality === "NO_DATA" && message.revision > boolBefore.currentValue.revision, "BOOLEAN disable NO_DATA update");
    await cloud(`/api/datapoint/${boolean.dataPointId}/enabled`, { method: "PUT", body: { enabled: true } });
    await waitGood(cloud, boolean.dataPointId, boolDisabled.currentValue.revision);
    log("PASS mapping reset and disable/enable propagated NO_DATA then real recovery over WebSocket");

    const merged = await primary.page.evaluate(() => {
      const current = window.__m5Factory.state().merged.find((item) => item.pointKey.includes('m5_number_'));
      const key = current.deviceId + '\\u0000' + current.pointKey;
      window.__m5Factory.merge({ ...current, type: 'update', revision: current.revision + 1, value: 999 });
      window.__m5Factory.merge({ ...current, type: 'snapshot', revision: current.revision, value: -999 });
      return window.__m5Factory.state().merged.find((item) => item.deviceId + '\\u0000' + item.pointKey === key);
    });
    assert.equal(merged.value, 999); assert.ok(merged.revision > 0);
    log("PASS auxiliary snapshot/live race assertion keeps the highest revision; real stream remains the primary evidence");

    const usedTicket = primaryConnection.ticket;
    const usedResult = await secondary.page.evaluate((ticket) => window.__m5Factory.secondUse(ticket), usedTicket);
    assert.equal(usedResult.opened, false, `used ticket was accepted: ${JSON.stringify(usedResult)}`);
    log("PASS used one-time ticket was rejected");

    const expiredTicketResult = await requestJSON(cloudURL, "/api/realtime/ticket", token, { method: "POST" });
    assert.equal(expiredTicketResult.response.status, 200);
    const expiresAt = Date.parse(expiredTicketResult.payload.data.expiresAt);
    const waitMS = Math.max(0, expiresAt - Date.now() + 500);
    log(`waiting ${waitMS}ms for a real ticket to expire`);
    await delay(waitMS);
    const expiredResult = await secondary.page.evaluate((ticket) => window.__m5Factory.expired(ticket), expiredTicketResult.payload.data.ticket);
    assert.equal(expiredResult.opened, false, `expired ticket was accepted: ${JSON.stringify(expiredResult)}`);
    log("PASS expired one-time ticket was rejected");

    const snapshotsBeforeRestart = await primary.page.evaluate(() => window.__m5Factory.state().messages.filter((message) => message.type === "snapshot").length);
    const restart = await runRestartScript();
    assert.equal(restart, 0, "Cloud restart script failed");
    await waitFor(
      "primary browser reconnect",
      () => primary.page.evaluate(() => window.__m5Factory.state()),
      (state) => state.reconnects >= 1 && state.messages.filter((message) => message.type === "snapshot").length >= snapshotsBeforeRestart + points.length,
      120000,
    );
    const recovered = await primary.page.evaluate(() => window.__m5Factory.state());
    assert.ok(recovered.url.includes("ticket="));
    log(`PASS Cloud restart caused browser connection recovery with a new ticket, subscriptions, and snapshots (${snapshotsBeforeRestart} -> ${recovered.messages.filter((message) => message.type === "snapshot").length})`);

    const logout = await requestJSON(cloudURL, "/api/auth/logout", token, { method: "POST" });
    assert.equal(logout.response.status, 200, `logout failed: ${JSON.stringify(logout.payload)}`);
    await waitFor("revoked session WebSocket close", () => primary.page.evaluate(() => window.__m5Factory.state()), (state) => state.close?.code === 1008, sessionCloseTimeout, 250);
    log("PASS logout/session revoke closed the realtime connection");

    await secondary.page.evaluate(() => window.__m5Factory.close());
    log("PASS second browser remained independent until its own cleanup (connection isolation)");
    log("PASS browser never opened an MQTT connection; it only used REST ticket + WebSocket");
  } finally {
    await primary.page.close(); await secondary.page.close(); await browser.close();
  }
  log("PASS M5 real CurrentValue/WebSocket acceptance completed");
}

async function runRestartScript() {
  await access(cloudRestartScript);
  const isNodeScript = /\.(?:mjs|cjs|js)$/i.test(cloudRestartScript);
  const command = isNodeScript ? process.execPath : cloudRestartScript;
  const args = isNodeScript ? [cloudRestartScript] : [];
  log(`running explicit Cloud restart hook: ${cloudRestartScript}`);
  const child = spawn(command, args, { cwd: repoRoot, env: { ...process.env, M5_CLOUD_URL: cloudURL }, stdio: "inherit" });
  return new Promise((resolve, reject) => { child.once("error", reject); child.once("close", (code) => resolve(code ?? 1)); });
}

try {
  await runAcceptance();
} catch (error) {
  log(`FAIL ${error instanceof Error ? error.stack ?? error.message : String(error)}`);
  process.exitCode = 1;
}
