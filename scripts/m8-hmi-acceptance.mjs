/*
 * M8 real HMI acceptance. This is deliberately opt-in and owns a unique
 * Compose project, temporary databases, child processes, browser pages, and
 * Modbus PTY alias. It follows ADR-0016 §25 and does not use M7 History/Event.
 *
 *   task m8:hmi-acceptance
 *   M8_REQUIRED=1 task m8:hmi-acceptance
 */
import assert from "node:assert/strict";
import { access, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { spawn, spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import { randomUUID } from "node:crypto";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";

const repoRoot = fileURLToPath(new URL("../", import.meta.url));
const serverDir = path.join(repoRoot, "server");
const webDir = path.join(repoRoot, "web");
const requireWebPackage = createRequire(path.join(webDir, "package.json"));
const collectorRoot = path.resolve(process.env.M8_COLLECTOR_ROOT ?? path.join(repoRoot, "..", "edge-collector"));
const collectorDir = path.basename(collectorRoot) === "edge-collector-api"
  ? collectorRoot
  : path.join(collectorRoot, "edge-collector-api");
const simulatorDir = path.join(collectorRoot, "modbus-simulator");
const composeFile = path.join(collectorRoot, "testdata/mqtt/docker-compose.yml");

const required = process.env.M8_REQUIRED === "1";
const cleanupEnabled = process.env.M8_CLEANUP !== "0";
const topicPrefix = process.env.M8_TOPIC_PREFIX ?? "edge";
const edgeID = process.env.M8_EDGE_ID ?? `m8-edge-${process.pid}`;
const sourceDeviceID = process.env.M8_SOURCE_DEVICE_ID ?? `m8-device-${process.pid}`;
const brokerPort = numberEnv("M8_BROKER_PORT", 18888);
const postgresPort = numberEnv("M8_POSTGRES_PORT", 15438);
const cloudPort = numberEnv("M8_CLOUD_PORT", 18218);
const collectorPort = numberEnv("M8_COLLECTOR_PORT", 18219);
const webPort = numberEnv("M8_WEB_PORT", 14218);
const composeProject = `m8-hmi-${process.pid}-${randomUUID().slice(0, 8)}`;
const cloudUser = process.env.M8_POSTGRES_USER ?? "m8_acceptance";
const cloudPassword = process.env.M8_POSTGRES_PASSWORD ?? "m8-acceptance-password";
const bootDatabase = safeIdentifier(process.env.M8_POSTGRES_BOOT_DATABASE ?? `m8_boot_${process.pid}`);
const cloudDatabase = safeIdentifier(process.env.M8_CLOUD_DATABASE ?? `m8_cloud_${process.pid}`);
const collectorDatabase = safeIdentifier(process.env.M8_COLLECTOR_DATABASE ?? `m8_collector_${process.pid}`);
if (new Set([bootDatabase, cloudDatabase, collectorDatabase]).size !== 3 || [bootDatabase, cloudDatabase, collectorDatabase].includes("postgres")) {
  throw new Error("M8 PostgreSQL boot, Cloud, and Collector database names must be distinct and cannot be 'postgres'");
}
const cloudURL = `http://127.0.0.1:${cloudPort}`;
const collectorURL = `http://127.0.0.1:${collectorPort}`;
const webURL = `http://127.0.0.1:${webPort}`;
const runID = randomUUID().replaceAll("-", "").slice(0, 12);
const numberPointKey = `m8_current_${runID}`;
const booleanPointKey = `m8_switch_${runID}`;
const sourceAlias = path.join(os.tmpdir(), `m8-hmi-${process.pid}-${runID}-rtu`);
const numberAddress = integerEnv("M8_NUMBER_ADDRESS", 100);
const booleanAddress = integerEnv("M8_BOOLEAN_ADDRESS", 102);
const booleanBit = integerEnv("M8_BOOLEAN_BIT", 0, 15);
const numberScale = finiteEnv("M8_NUMBER_SCALE", 1);

const children = new Set();
let tempRoot;
let composeStarted = false;
let aliasCreated = false;
let cloudProcess;
let collectorProcess;
let simulatorProcess;
let webProcess;
let cloudBinary;
let cloudMigrateBinary;
let collectorBinary;
let collectorMigrateBinary;
let cloudEnvironment;
let collectorEnvironment;
let chromium;
let browser;
let runtimePage;
let restrictedRuntimePage;
let cloudToken;
let collectorToken;

function numberEnv(name, fallback) {
  const value = Number(process.env[name] ?? fallback);
  if (!Number.isInteger(value) || value < 1 || value > 65535) throw new Error(`${name} must be a TCP port in [1, 65535]`);
  return value;
}

function integerEnv(name, fallback, max = 65535) {
  const value = Number(process.env[name] ?? fallback);
  if (!Number.isInteger(value) || value < 0 || value > max) throw new Error(`${name} must be an integer in [0, ${max}]`);
  return value;
}

function finiteEnv(name, fallback) {
  const value = Number(process.env[name] ?? fallback);
  if (!Number.isFinite(value)) throw new Error(`${name} must be finite`);
  return value;
}

function safeIdentifier(value) {
  const normalized = String(value).toLowerCase();
  if (!/^[a-z0-9_]{1,48}$/.test(normalized)) throw new Error("M8 database names may contain only lowercase letters, digits, and underscore");
  return normalized;
}

function redact(value) {
  return String(value)
    .replaceAll(cloudPassword, "<redacted-password>")
    .replaceAll(process.env.M8_JWT_SECRET ?? "m8-cloud-jwt-secret", "<redacted-jwt>")
    .replaceAll(process.env.M8_COLLECTOR_JWT_SECRET ?? "m8-collector-jwt-secret", "<redacted-jwt>");
}

function log(message) { process.stdout.write(`[m8-hmi] ${message}\n`); }

function hasCommand(command, args = ["--version"]) {
  return spawnSync(command, args, { stdio: "ignore" }).status === 0;
}

function loadPlaywrightChromium() {
  const playwright = requireWebPackage("playwright");
  const resolved = playwright.chromium ?? playwright.default?.chromium;
  if (!resolved || typeof resolved.executablePath !== "function") throw new Error("resolved Playwright package does not expose chromium");
  return resolved;
}

function startProcess(command, args, cwd, environment = {}) {
  const child = spawn(command, args, { cwd, env: { ...process.env, ...environment }, detached: true, stdio: ["ignore", "pipe", "pipe"] });
  child.output = "";
  const collect = (chunk) => { child.output = `${child.output}${chunk}`.slice(-120000); };
  child.stdout?.on("data", collect);
  child.stderr?.on("data", collect);
  children.add(child);
  return child;
}

function signalProcessGroup(child, signal) {
  if (!child?.pid) return;
  try { process.kill(-child.pid, signal); } catch { /* process group has exited */ }
  try { process.kill(child.pid, signal); } catch { /* process has exited */ }
}

async function stopProcess(child) {
  if (!child) return;
  if (child.exitCode === null && child.signalCode === null) {
    const closed = new Promise((resolve) => child.once("close", resolve));
    signalProcessGroup(child, "SIGTERM");
    await Promise.race([closed, delay(5000)]);
    if (child.exitCode === null && child.signalCode === null) {
      const killed = new Promise((resolve) => child.once("close", resolve));
      signalProcessGroup(child, "SIGKILL");
      await Promise.race([killed, delay(2000)]);
    }
  }
  children.delete(child);
}

async function runCommand(command, args, cwd, environment = {}) {
  const child = startProcess(command, args, cwd, environment);
  const result = await new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("close", (code, signal) => resolve({ code, signal }));
  });
  children.delete(child);
  if (result.code !== 0) throw new Error(`${command} ${args.join(" ")} failed (${result.code ?? result.signal}):\n${redact(child.output)}`);
  return child.output;
}

const composeEnvironment = {
  MQTT_E2E_BROKER_PORT: String(brokerPort),
  MQTT_E2E_POSTGRES_PORT: String(postgresPort),
  MQTT_E2E_POSTGRES_DB: bootDatabase,
  MQTT_E2E_POSTGRES_USER: cloudUser,
  MQTT_E2E_POSTGRES_PASSWORD: cloudPassword,
};

function composeArgs(...args) { return ["compose", "--project-name", composeProject, "--file", composeFile, ...args]; }
async function compose(...args) { return runCommand("docker", composeArgs(...args), collectorRoot, composeEnvironment); }

async function waitForPort(host, port, timeout = 60000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const open = await new Promise((resolve) => {
      const socket = net.createConnection({ host, port });
      socket.once("connect", () => { socket.destroy(); resolve(true); });
      socket.once("error", () => { socket.destroy(); resolve(false); });
    });
    if (open) return;
    await delay(200);
  }
  throw new Error(`TCP endpoint did not become ready: ${host}:${port}`);
}

async function waitForHTTP(url, child, timeout = 90000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(2000) });
      if (response.ok) return;
    } catch { /* still starting */ }
    if (child && child.exitCode !== null) throw new Error(`process exited waiting for ${url}:\n${redact(child.output)}`);
    await delay(250);
  }
  throw new Error(`endpoint did not become ready: ${url}\n${redact(child?.output ?? "")}`);
}

async function waitForHTTPDown(url, timeout = 15000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try { await fetch(url, { signal: AbortSignal.timeout(500) }); } catch { return; }
    await delay(100);
  }
  throw new Error(`endpoint remained reachable: ${url}`);
}

async function eventually(label, read, predicate, timeout = 90000, interval = 250) {
  const deadline = Date.now() + timeout;
  let value;
  while (Date.now() < deadline) {
    value = await read();
    if (predicate(value)) return value;
    await delay(interval);
  }
  throw new Error(`${label} timed out: ${JSON.stringify(value)}`);
}

async function waitComposeHealthy(service, timeout = 90000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const output = await compose("ps", "--format", "json", service);
    const records = output.split("\n").map((line) => line.trim()).filter(Boolean).flatMap((line) => {
      try { const value = JSON.parse(line); return Array.isArray(value) ? value : [value]; } catch { return []; }
    });
    if (records.some((record) => record.Service === service && (record.Health === "healthy" || String(record.Status ?? "").includes("(healthy)")))) return;
    await delay(500);
  }
  throw new Error(`Compose service did not become healthy: ${service}`);
}

function databaseURL(database) { return `postgres://127.0.0.1:${postgresPort}/${database}?sslmode=disable`; }
function sqlLiteral(value) { return `'${String(value).replaceAll("'", "''")}'`; }
function sqlIdentifier(value) { return `"${String(value).replaceAll('"', '""')}"`; }

async function postgresQuery(database, query) {
  return compose("exec", "-T", "postgres", "env", `PGPASSWORD=${cloudPassword}`, "psql", "-h", "127.0.0.1", "-U", cloudUser, "-d", database, "-At", "-v", "ON_ERROR_STOP=1", "-c", query);
}

async function createDatabase(database) {
  const existing = (await postgresQuery("postgres", `SELECT 1 FROM pg_database WHERE datname=${sqlLiteral(database)}`)).trim();
  if (!existing) {
    await compose("exec", "-T", "postgres", "env", `PGPASSWORD=${cloudPassword}`, "psql", "-h", "127.0.0.1", "-U", cloudUser, "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", `CREATE DATABASE ${sqlIdentifier(database)} OWNER ${sqlIdentifier(cloudUser)}`);
  }
}

function cloudEnv() {
  return {
    APP_ENV: "dev", APP_DATABASE__DRIVER: "postgres", APP_DATABASE__URL: databaseURL(cloudDatabase),
    APP_DATABASE__USERNAME: cloudUser, APP_DATABASE__PASSWORD: cloudPassword,
    APP_HTTP__ADDRESS: `127.0.0.1:${cloudPort}`, APP_JWT__SECRET: process.env.M8_JWT_SECRET ?? "m8-cloud-jwt-secret",
    APP_FILE__STORAGE_ROOT: path.join(tempRoot, "cloud-files"), APP_LOG__LEVEL: "info", APP_LOG__FORMAT: "text",
    APP_MQTT__ENABLED: "true", APP_MQTT__URL: `mqtt://127.0.0.1:${brokerPort}`, APP_MQTT__PROTOCOL: "mqtt5",
    APP_MQTT__CLIENT_ID: `m8-cloud-${process.pid}`, APP_MQTT__PREFIX: topicPrefix, APP_MQTT__USERNAME: "", APP_MQTT__PASSWORD: "",
    GOCACHE: path.join(tempRoot, "go-cache-cloud"),
  };
}

function collectorEnv() {
  return {
    APP_ENV: "dev", APP_DATABASE__DRIVER: "postgres", APP_DATABASE__URL: databaseURL(collectorDatabase),
    APP_DATABASE__USERNAME: cloudUser, APP_DATABASE__PASSWORD: cloudPassword,
    APP_HTTP__ADDRESS: `127.0.0.1:${collectorPort}`, APP_JWT__SECRET: process.env.M8_COLLECTOR_JWT_SECRET ?? "m8-collector-jwt-secret",
    APP_FILE__STORAGE_ROOT: path.join(tempRoot, "collector-files"), APP_LOG__LEVEL: "info", APP_LOG__FORMAT: "text",
    GOCACHE: path.join(tempRoot, "go-cache-collector"),
  };
}

async function startInfrastructure() {
  composeStarted = true;
  await compose("up", "-d", "--remove-orphans", "postgres", "broker");
  await waitForPort("127.0.0.1", postgresPort);
  await waitForPort("127.0.0.1", brokerPort);
  await waitComposeHealthy("postgres");
  await waitComposeHealthy("broker");
  await createDatabase(cloudDatabase);
  await createDatabase(collectorDatabase);
  log(`owned PostgreSQL/Mosquitto project ready (${composeProject})`);
}

async function buildAndMigrate() {
  cloudEnvironment = cloudEnv();
  collectorEnvironment = collectorEnv();
  cloudBinary = path.join(tempRoot, "edge-platform-api");
  cloudMigrateBinary = path.join(tempRoot, "edge-platform-migrate");
  collectorBinary = path.join(tempRoot, "edge-collector-api");
  collectorMigrateBinary = path.join(tempRoot, "edge-collector-migrate");
  await runCommand("go", ["build", "-o", cloudBinary, "./cmd/api"], serverDir, cloudEnvironment);
  await runCommand("go", ["build", "-o", cloudMigrateBinary, "./cmd/migrate"], serverDir, cloudEnvironment);
  await runCommand("go", ["build", "-o", collectorBinary, "./cmd/api"], collectorDir, collectorEnvironment);
  await runCommand("go", ["build", "-o", collectorMigrateBinary, "./cmd/migrate"], collectorDir, collectorEnvironment);
  await runCommand(cloudMigrateBinary, ["up", "--kind", "all"], serverDir, cloudEnvironment);
  await runCommand(collectorMigrateBinary, ["up", "--kind", "all"], collectorDir, collectorEnvironment);
}

async function writeSimulatorConfig() {
  try {
    await access(sourceAlias);
    throw new Error(`refusing to replace pre-existing PTY alias ${sourceAlias}`);
  } catch (error) {
    if (error?.code !== "ENOENT") throw error;
  }
  const configRoot = path.join(tempRoot, "modbus-simulator");
  const devicesRoot = path.join(configRoot, "devices");
  await mkdir(devicesRoot, { recursive: true });
  await writeFile(path.join(devicesRoot, "time_registers_03.yaml"), await readFile(path.join(simulatorDir, "config/devices/time_registers_03.yaml"), "utf8"));
  const configPath = path.join(configRoot, "m8-hmi.yaml");
  await writeFile(configPath, `logging:\n  level: INFO\n  hex: false\n\nchannels:\n  - name: m8-hmi-rtu\n    protocol: rtu\n    alias: ${sourceAlias}\n    baudrate: 9600\n    bytesize: 8\n    parity: N\n    stopbits: 1\n    devices:\n      - devices/time_registers_03.yaml\n`);
  return configPath;
}

async function waitForOutput(child, expected, timeout = 60000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if (child.output.includes(expected)) return;
    if (child.exitCode !== null) throw new Error(`process exited before ${expected}:\n${redact(child.output)}`);
    await delay(100);
  }
  throw new Error(`process did not emit ${expected}:\n${redact(child.output)}`);
}

async function waitForPath(file, timeout = 30000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try { await access(file); aliasCreated = true; return; } catch { await delay(100); }
  }
  throw new Error(`path did not become ready: ${file}`);
}

async function startSimulator() {
  const configPath = await writeSimulatorConfig();
  simulatorProcess = startProcess("uv", ["run", "modbus-simulator", "--config", configPath], simulatorDir, { UV_CACHE_DIR: path.join(tempRoot, "uv-cache") });
  await waitForOutput(simulatorProcess, "Modbus Simulator Started");
  await waitForPath(sourceAlias);
  log("real Modbus simulator PTY is ready");
}

async function startCloud() {
  cloudProcess = startProcess(cloudBinary, [], serverDir, cloudEnvironment);
  await waitForHTTP(`${cloudURL}/health`, cloudProcess);
}

async function startCollector() {
  collectorProcess = startProcess(collectorBinary, [], collectorDir, collectorEnvironment);
  await waitForHTTP(`${collectorURL}/health`, collectorProcess);
}

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

async function login(baseURL, username = "admin", password = "admin123") {
  const result = await requestJSON(baseURL, "/api/auth/login", "", { method: "POST", body: { username, password } });
  assert.equal(result.response.status, 200, `login failed at ${baseURL}: ${JSON.stringify(result.payload)}`);
  assert.equal(result.payload.code, 200, `login failed at ${baseURL}: ${JSON.stringify(result.payload)}`);
  assert.ok(result.payload.data?.tokenValue, "login returned no token");
  return result.payload.data.tokenValue;
}

async function requestAPI(baseURL, token, pathname, options = {}) {
  return requestJSON(baseURL, pathname, token, options);
}

async function api(baseURL, token, pathname, options = {}) {
  const result = await requestAPI(baseURL, token, pathname, options);
  assert.equal(result.response.status, 200, `${options.method ?? "GET"} ${pathname}: ${JSON.stringify(result.payload)}`);
  assert.equal(result.payload.code, 200, `${options.method ?? "GET"} ${pathname}: ${JSON.stringify(result.payload)}`);
  return result.payload.data;
}

async function collectorAPI(pathname, options = {}) { return api(collectorURL, collectorToken, pathname, options); }

async function configureCollector() {
  await collectorAPI("/api/v1/mqtt/config", {
    method: "PUT",
    body: {
      enabled: true, edgeId: edgeID, brokerUrl: `mqtt://127.0.0.1:${brokerPort}`, protocolVersion: "MQTT_5",
      clientId: `m8-collector-${process.pid}`, username: "", passwordAction: "clear", password: "", tlsEnabled: false,
      caCertificate: "", clientCertificate: "", clientPrivateKeyAction: "clear", clientPrivateKey: "", keepAliveSeconds: 5,
      connectTimeoutMs: 3000, reconnectMinMs: 200, reconnectMaxMs: 1000, topicPrefix, rawPublishIntervalMs: 250,
      outboxMaxRows: 100, outboxMaxBytes: 8 * 1024 * 1024, outboxRetentionDays: 7, commandJournalRetentionDays: 7,
      commandJournalMaxRows: 100, commandQueueCapacity: 8, commandPollFairness: 1,
    },
  });
  await eventually("Collector MQTT connected", () => collectorAPI("/api/v1/mqtt/state"), (state) => state.state === "CONNECTED" && state.connected === true, 45000);
}

function commandSource() {
  return `def after_poll(ctx):
    hour = ctx.raw_register(3, 100)
    minute = ctx.raw_register(3, 101)
    second = ctx.raw_register(3, 102)
    if hour == None or minute == None or second == None:
        return
    return

def command(ctx, name, args):
    if name != "set_clock":
        return None
    ctx.delay(5000)
    ctx.write_registers(100, [args["hour"], args["minute"], args["second"]])
    return {"written": [args["hour"], args["minute"], args["second"]]}
`;
}

async function createCollectorFixture() {
  const script = await collectorAPI("/api/v1/acquisition/scripts", {
    method: "POST", body: { name: `M8 HMI acceptance ${runID}`, description: "real HMI Modbus control fixture", draftSource: commandSource() },
  });
  await collectorAPI(`/api/v1/acquisition/scripts/${script.id}/validate`, { method: "POST" });
  await collectorAPI(`/api/v1/acquisition/scripts/${script.id}/publish`, { method: "POST" });
  const channel = await collectorAPI("/api/v1/acquisition/channels", {
    method: "POST",
    body: { name: `M8 HMI RTU ${runID}`, protocol: "MODBUS_RTU", serialConfig: { port: sourceAlias, baudRate: 9600, dataBits: 8, stopBits: 1, parity: "N" }, timeoutMs: 500, interRequestDelayMs: 5, enabled: 1 },
  });
  const device = await collectorAPI("/api/v1/acquisition/devices", {
    method: "POST",
    body: { externalId: sourceDeviceID, name: "M8 HMI Modbus simulator", deviceType: "FEED_PROTECTOR", channelId: channel.id, unitId: 3, pollIntervalMs: 250, failureThreshold: 3, enabled: 1, registerBlocks: [{ name: "hmi-raw", functionCode: 3, startAddress: 100, quantity: 3, sortOrder: 0 }] },
  });
  await collectorAPI(`/api/v1/acquisition/devices/${device.id}/script`, { method: "PUT", body: { scriptId: script.id } });
  await eventually("Collector Modbus device ONLINE", () => collectorAPI(`/api/v1/acquisition/states/${device.id}`), (state) => state.status === "ONLINE" && state.registerBlocks?.every((block) => block.valid), 60000);
  return { script, channel, device };
}

async function findCloudDevice() {
  return eventually("Cloud Device discovery", async () => {
    const page = await api(cloudURL, cloudToken, `/api/device/page?page=1&pageSize=100&edgeId=${encodeURIComponent(edgeID)}&sourceDeviceId=${encodeURIComponent(sourceDeviceID)}`);
    return page.records?.find((record) => record.edgeId === edgeID && record.sourceDeviceId === sourceDeviceID);
  }, Boolean, 60000);
}

function mapping(valueType) {
  if (valueType === "BOOLEAN") return { sourceType: "MODBUS_REGISTER", functionCode: 3, address: booleanAddress, encoding: "BOOLEAN_BIT", byteOrder: "BIG_ENDIAN", wordOrder: null, bitIndex: booleanBit, scale: 1, offset: 0 };
  return { sourceType: "MODBUS_REGISTER", functionCode: 3, address: numberAddress, encoding: "UINT16", byteOrder: "BIG_ENDIAN", wordOrder: null, bitIndex: null, scale: numberScale, offset: 0 };
}

async function createPoint(deviceID, valueType, pointKey) {
  return api(cloudURL, cloudToken, "/api/datapoint", {
    method: "POST",
    body: { deviceId: deviceID, pointKey, name: `M8 ${valueType} ${runID}`, valueType, unit: valueType === "NUMBER" ? "A" : null, precision: valueType === "NUMBER" ? 1 : null, mapping: mapping(valueType) },
  });
}

async function current(pointID) { return api(cloudURL, cloudToken, `/api/datapoint/${encodeURIComponent(pointID)}`); }
async function waitPoint(pointID, predicate, label) { return eventually(label, () => current(pointID), predicate, 90000, 250); }
async function waitGood(pointID, revision = 0) { return waitPoint(pointID, (point) => point.currentValue.quality === "GOOD" && point.currentValue.revision > revision, "real M4 CurrentValue GOOD"); }
async function waitQuality(pointID, quality, revision) { return waitPoint(pointID, (point) => point.currentValue.quality === quality && point.currentValue.revision > revision, `real M4 CurrentValue ${quality}`); }

async function updateCollectorChannel(channel, serialPort) {
  return collectorAPI(`/api/v1/acquisition/channels/${encodeURIComponent(channel.id)}`, {
    method: "PUT",
    body: { name: channel.name, protocol: channel.protocol, serialConfig: { ...channel.serialConfig, port: serialPort }, timeoutMs: channel.timeoutMs, interRequestDelayMs: channel.interRequestDelayMs, enabled: channel.enabled },
  });
}

async function waitCommand(commandID, predicate, label) {
  return eventually(label, async () => {
    const result = await requestAPI(cloudURL, cloudToken, `/api/command/${encodeURIComponent(commandID)}`);
    return result.response.status === 200 && result.payload.code === 200 ? result.payload.data : null;
  }, predicate, 90000, 250);
}

function datapointBinding(point) { return { kind: "datapoint", deviceId: point.deviceId, pointKey: point.pointKey }; }
function commandBinding(deviceID, args) {
  return { kind: "command", deviceId: deviceID, name: "set_clock", args, ttlSeconds: 30, confirmation: { required: true, message: "Confirm real Modbus write?" } };
}

function hmiNode(type, x, y, width, height, props, bindings = {}) {
  return { nodeId: randomUUID(), x, y, width, height, rotation: 0, zIndex: y + x, type, props, bindings };
}

function runtimeDocument(deviceID, numberPoint, booleanPoint) {
  const button = commandBinding(deviceID, { hour: 7, minute: 8, second: 1 });
  const switchOn = commandBinding(deviceID, { hour: 7, minute: 8, second: 1 });
  const switchOff = commandBinding(deviceID, { hour: 7, minute: 8, second: 0 });
  return {
    schema: "hmi-page/v1",
    canvas: { width: 1280, height: 720 },
    nodes: [
      hmiNode("text", 32, 24, 400, 48, { text: "published-runtime-label", color: "#1f2937", fontSize: 24, fontWeight: "bold", align: "left" }),
      hmiNode("value-display", 32, 96, 280, 96, { label: "Number Current", precision: 1, showUnit: true, color: "#1f2937" }, { value: datapointBinding(numberPoint) }),
      hmiNode("gauge", 340, 96, 280, 190, { label: "Number Gauge", min: 0, max: 24, precision: 1, showUnit: true }, { value: datapointBinding(numberPoint) }),
      hmiNode("indicator", 650, 96, 260, 96, { label: "Boolean Indicator", trueLabel: "开", falseLabel: "关", trueColor: "success", falseColor: "neutral" }, { value: datapointBinding(booleanPoint) }),
      hmiNode("switch", 940, 96, 280, 72, { label: "现场开关", onLabel: "开", offLabel: "关" }, { state: datapointBinding(booleanPoint), onCommand: switchOn, offCommand: switchOff }),
      hmiNode("button", 32, 240, 280, 72, { label: "设置现场开" }, { command: button }),
      hmiNode("value-display", 32, 360, 280, 96, { label: "Duplicate Number", precision: 1, showUnit: true, color: "#1f2937" }, { value: datapointBinding(numberPoint) }),
    ],
  };
}

async function hmiRequest(pathname, options = {}) { return requestAPI(cloudURL, cloudToken, pathname, options); }
async function createPage(name, document) {
  return api(cloudURL, cloudToken, "/api/hmi/page", { method: "POST", body: { name, description: "M8 acceptance fixture", draftDocument: document } });
}
async function saveDraft(pageID, revision, document) {
  return hmiRequest(`/api/hmi/page/${encodeURIComponent(pageID)}/draft`, { method: "PUT", body: { expectedDraftRevision: revision, document } });
}
async function publishPage(pageID, revision) {
  return hmiRequest(`/api/hmi/page/${encodeURIComponent(pageID)}/publish`, { method: "POST", body: { expectedDraftRevision: revision } });
}
async function getRuntime(pageID, token = cloudToken) { return requestAPI(cloudURL, token, `/api/hmi/page/${encodeURIComponent(pageID)}/runtime`); }

async function assertVersionCount(pageID, count) {
  const actual = Number((await postgresQuery(cloudDatabase, `SELECT COUNT(*) FROM hmi_page_version WHERE page_id=${sqlLiteral(pageID)}`)).trim());
  assert.equal(actual, count, `page ${pageID} has unexpected immutable-version count`);
}

async function createRunOnlyUser() {
  const username = `m8-run-only-${runID}`;
  await api(cloudURL, cloudToken, "/api/system/user", { method: "POST", body: { username, nickname: "M8 runtime only", gender: "UNSPECIFIED", status: 1 } });
  const users = await api(cloudURL, cloudToken, `/api/system/user/page?page=1&pageSize=500&username=${encodeURIComponent(username)}`);
  const user = users.records.find((record) => record.username === username);
  assert.ok(user?.id, "created run-only user was not returned by user management");

  const roleCode = `M8RUN${runID.toUpperCase()}`;
  await api(cloudURL, cloudToken, "/api/system/role", { method: "POST", body: { roleName: `M8 run only ${runID}`, roleCode, status: 1, sortOrder: 999 } });
  const roles = await api(cloudURL, cloudToken, `/api/system/role/page?page=1&pageSize=500&roleCode=${encodeURIComponent(roleCode)}`);
  const role = roles.records.find((record) => record.roleCode === roleCode);
  assert.ok(role?.id, "created run-only role was not returned by role management");
  const menus = await api(cloudURL, cloudToken, "/api/system/menu/page?page=1&pageSize=500");
  const runMenu = menus.records.find((record) => record.permissionCode === "hmi:run");
  assert.ok(runMenu?.id, "hmi:run permission menu was not seeded");
  await api(cloudURL, cloudToken, `/api/system/role/${role.id}/menus`, { method: "PUT", body: { menuIds: [runMenu.id] } });
  await api(cloudURL, cloudToken, `/api/system/user/${user.id}/roles`, { method: "PUT", body: { roleIds: [role.id] } });
  return { user, token: await login(cloudURL, username, "admin123") };
}

async function assertLifecycle(deviceID, numberPoint, booleanPoint) {
  const draft = { schema: "hmi-page/v1", canvas: { width: 1280, height: 720 }, nodes: [] };
  const page = await createPage(`M8 Runtime ${runID}`, draft);
  const pageID = page.pageId;
  assert.ok(pageID);
  assert.equal(page.draftRevision, 1);
  const unpublished = await getRuntime(pageID);
  assert.equal(unpublished.response.status, 404, "runtime must not fall back to draft before the first publish");

  const document = runtimeDocument(deviceID, numberPoint, booleanPoint);
  const saved = await saveDraft(pageID, 1, document);
  assert.equal(saved.response.status, 200, JSON.stringify(saved.payload));
  const revision2 = saved.payload.data;
  assert.equal(revision2.draftRevision, 2);
  const firstPublish = await publishPage(pageID, 2);
  assert.equal(firstPublish.response.status, 200, JSON.stringify(firstPublish.payload));
  assert.equal(firstPublish.payload.data.versionNo, 1);
  const publishedVersionID = firstPublish.payload.data.versionId;
  await assertVersionCount(pageID, 1);

  const draftOnlyDocument = structuredClone(document);
  draftOnlyDocument.nodes[0].props.text = "draft-only-label";
  const draftUpdate = await saveDraft(pageID, 2, draftOnlyDocument);
  assert.equal(draftUpdate.response.status, 200, JSON.stringify(draftUpdate.payload));
  assert.equal(draftUpdate.payload.data.draftRevision, 3);
  const runtimeAfterDraft = await getRuntime(pageID);
  assert.equal(runtimeAfterDraft.response.status, 200, JSON.stringify(runtimeAfterDraft.payload));
  assert.equal(runtimeAfterDraft.payload.data.version.versionId, publishedVersionID);
  assert.equal(runtimeAfterDraft.payload.data.version.document.nodes[0].props.text, "published-runtime-label");
  assert.equal(runtimeAfterDraft.payload.data.dataPoints.length, 2, "runtime bootstrap must deduplicate repeated DataPoint bindings");

  const staleSave = await saveDraft(pageID, 2, document);
  assert.equal(staleSave.response.status, 409, JSON.stringify(staleSave.payload));
  const retryPublish = await publishPage(pageID, 2);
  assert.equal(retryPublish.response.status, 200, JSON.stringify(retryPublish.payload));
  assert.equal(retryPublish.payload.data.versionId, publishedVersionID);
  assert.equal(retryPublish.payload.data.versionNo, 1);
  await assertVersionCount(pageID, 1);

  const invalid = await createPage(`M8 Invalid binding ${runID}`, draft);
  const invalidDocument = structuredClone(document);
  const gauge = invalidDocument.nodes.find((node) => node.type === "gauge");
  gauge.bindings.value = datapointBinding(booleanPoint);
  const invalidSave = await saveDraft(invalid.pageId, 1, invalidDocument);
  assert.equal(invalidSave.response.status, 200, JSON.stringify(invalidSave.payload));
  const invalidPublish = await publishPage(invalid.pageId, invalidSave.payload.data.draftRevision);
  assert.equal(invalidPublish.response.status, 400, JSON.stringify(invalidPublish.payload));
  const rejectedRuntime = await getRuntime(invalid.pageId);
  assert.equal(rejectedRuntime.response.status, 404, "invalid binding must not produce a runtime version");

  log(JSON.stringify({ scenario: "lifecycle-isolation-revision-idempotency-binding-validation", status: "PASS", pageId: pageID, versionId: publishedVersionID, duplicateBindings: document.nodes.filter((node) => node.bindings.value?.kind === "datapoint").length }));
  return { pageID, versionID: publishedVersionID, document };
}

async function configureBrowserProbe(page) {
  await page.addInitScript(() => {
    const NativeWebSocket = window.WebSocket;
    window.__m8Probe = { sockets: [], frames: [], messages: [] };
    window.WebSocket = class extends NativeWebSocket {
      constructor(...args) {
        super(...args);
        window.__m8Probe.sockets.push(this);
        window.__m8Probe.urls = [...(window.__m8Probe.urls ?? []), String(args[0])];
        this.addEventListener("message", (event) => {
          try { window.__m8Probe.messages.push(JSON.parse(event.data)); } catch { /* ignore non-JSON */ }
        });
      }
      send(data) {
        try { window.__m8Probe.frames.push(JSON.parse(data)); } catch { /* ignore non-JSON */ }
        return super.send(data);
      }
    };
  });
}

async function launchBrowser() {
  const executablePath = process.env.M8_PLAYWRIGHT_EXECUTABLE_PATH ?? chromium.executablePath();
  await access(executablePath);
  return chromium.launch({ headless: true, executablePath });
}

async function openRuntimeBrowser(pageID, username = "admin", password = "admin123") {
  const page = await browser.newPage({ viewport: { width: 1440, height: 960 } });
  await configureBrowserProbe(page);
  await page.goto(`${webURL}/login`, { waitUntil: "networkidle" });
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码").fill(password);
  await page.getByRole("button", { name: "登录" }).click();
  await page.waitForURL(/\/dashboard/);
  await page.goto(`${webURL}/hmi/pages/${encodeURIComponent(pageID)}/run`, { waitUntil: "networkidle" });
  await page.getByRole("heading", { name: `M8 Runtime ${runID}`, exact: true }).waitFor();
  await page.getByText("实时数据已连接", { exact: true }).waitFor({ timeout: 30000 });
  return page;
}

async function readProbe(page) { return page.evaluate(() => ({ urls: window.__m8Probe?.urls ?? [], frames: window.__m8Probe?.frames ?? [], messages: window.__m8Probe?.messages ?? [], socketCount: window.__m8Probe?.sockets.length ?? 0 })); }

async function assertBrowserUsesCloudRealtime(page, numberPoint, booleanPoint) {
  const probe = await readProbe(page);
  assert.ok(probe.urls.length >= 1, "runtime did not create a WebSocket");
  for (const rawURL of probe.urls) {
    const url = new URL(rawURL);
    assert.equal(url.host, `127.0.0.1:${cloudPort}`, `browser opened a non-Cloud WebSocket ${url.host}`);
    assert.equal(url.pathname, "/api/realtime/ws");
    assert.equal(url.searchParams.has("token"), false, "browser exposed a long-lived bearer token in the WebSocket URL");
  }
  const subscriptions = probe.frames.filter((frame) => frame.type === "subscribe");
  assert.ok(subscriptions.length >= 1, "runtime did not batch-subscribe points");
  const first = subscriptions[0].points;
  assert.equal(first.length, 2, "duplicate HMI bindings must produce one subscription per unique point");
  assert.deepEqual(new Set(first.map((point) => `${point.deviceId}\u0000${point.pointKey}`)), new Set([`${numberPoint.deviceId}\u0000${numberPoint.pointKey}`, `${booleanPoint.deviceId}\u0000${booleanPoint.pointKey}`]));
}

async function waitCommandFeedback(page, message) {
  await page.getByText(message, { exact: false }).first().waitFor({ timeout: 30000 });
}

async function assertRuntimeQualityAndControls(page, pageID, numberPoint, booleanPoint, fixture) {
  await assertBrowserUsesCloudRealtime(page, numberPoint, booleanPoint);
  const numberLabel = page.getByText("Number Current", { exact: true }).first();
  await eventually("HMI NUMBER value-display render", async () => numberLabel.locator("..").textContent(), (text) => /0(?:\.0)?/.test(text ?? ""), 30000, 250);
  await page.getByText("Boolean Indicator", { exact: true }).waitFor();
  await page.getByText("Number Gauge", { exact: true }).waitFor();

  // Draft writes must not change the already-open Published Runtime.
  await page.getByText("published-runtime-label", { exact: true }).waitFor();
  assert.equal(await page.getByText("draft-only-label", { exact: true }).count(), 0);

  const beforeBadNumber = await current(numberPoint.dataPointId);
  const beforeBadBoolean = await current(booleanPoint.dataPointId);
  const unavailablePort = path.join(os.tmpdir(), `m8-hmi-${process.pid}-${runID}-missing-serial`);
  assert.notEqual(unavailablePort, fixture.channel.serialConfig.port);
  await updateCollectorChannel(fixture.channel, unavailablePort);
  let badNumber;
  let badBoolean;
  try {
    badNumber = await waitQuality(numberPoint.dataPointId, "BAD", beforeBadNumber.currentValue.revision);
    badBoolean = await waitQuality(booleanPoint.dataPointId, "BAD", beforeBadBoolean.currentValue.revision);
    assert.equal(badNumber.currentValue.value, beforeBadNumber.currentValue.value, "BAD NUMBER did not retain the prior value");
    assert.equal(badBoolean.currentValue.value, beforeBadBoolean.currentValue.value, "BAD BOOLEAN did not retain the prior value");
    await page.getByText("数据无效", { exact: true }).first().waitFor();
    const probeBeforeDisconnect = await readProbe(page);
    const beforeSocketCount = probeBeforeDisconnect.socketCount;
    const beforeSnapshotCount = probeBeforeDisconnect.messages.filter((message) => message.type === "snapshot").length;
    await page.evaluate(() => window.__m8Probe.sockets.at(-1)?.close(4000, "M8 acceptance disconnect"));
    await page.getByText(/实时连接中断|实时数据连接已断开/, { exact: false }).waitFor({ timeout: 5000 });
    await page.waitForFunction((count) => window.__m8Probe.sockets.length > count, beforeSocketCount, { timeout: 30000 });
    await page.getByText("实时数据已连接", { exact: true }).waitFor({ timeout: 30000 });
    await page.waitForFunction((count) => window.__m8Probe.messages.filter((message) => message.type === "snapshot").length > count, beforeSnapshotCount, { timeout: 30000 });
    const badDuringDisconnect = await current(numberPoint.dataPointId);
    assert.equal(badDuringDisconnect.currentValue.quality, "BAD", "WebSocket disconnect must not rewrite point quality");
    assert.equal(badDuringDisconnect.currentValue.value, badNumber.currentValue.value);
    assert.ok((await readProbe(page)).messages.some((message) => message.type === "snapshot" && message.pointKey === numberPoint.pointKey && message.quality === "BAD"), "reconnect snapshot did not restore BAD CurrentValue");
    log(JSON.stringify({ scenario: "bad-quality-disconnect-and-reconnect-snapshot", status: "PASS", numberQuality: badDuringDisconnect.currentValue.quality, valueRetained: true }));
  } finally {
    await updateCollectorChannel(fixture.channel, fixture.channel.serialConfig.port);
  }
  await waitGood(numberPoint.dataPointId, badNumber.currentValue.revision);
  await waitGood(booleanPoint.dataPointId, badBoolean.currentValue.revision);

  const disabled = await api(cloudURL, cloudToken, `/api/datapoint/${encodeURIComponent(booleanPoint.dataPointId)}/enabled`, { method: "PUT", body: { enabled: false } });
  assert.equal(disabled.currentValue.quality, "NO_DATA");
  assert.equal(disabled.currentValue.value, null);
  await page.getByText("无数据", { exact: true }).waitFor({ timeout: 30000 });
  await page.getByText("--", { exact: true }).first().waitFor({ timeout: 30000 });
  const noDataRevision = disabled.currentValue.revision;
  const noDataSnapshotCount = (await readProbe(page)).messages.filter((message) => message.type === "snapshot").length;
  await page.evaluate(() => window.__m8Probe.sockets.at(-1)?.close(4000, "M8 NO_DATA disconnect"));
  await page.getByText("实时数据已连接", { exact: true }).waitFor({ timeout: 30000 });
  await page.waitForFunction((count) => window.__m8Probe.messages.filter((message) => message.type === "snapshot").length > count, noDataSnapshotCount, { timeout: 30000 });
  const noDataAfterReconnect = await current(booleanPoint.dataPointId);
  assert.equal(noDataAfterReconnect.currentValue.quality, "NO_DATA");
  assert.ok(noDataAfterReconnect.currentValue.revision >= noDataRevision);
  await api(cloudURL, cloudToken, `/api/datapoint/${encodeURIComponent(booleanPoint.dataPointId)}/enabled`, { method: "PUT", body: { enabled: true } });
  await waitGood(booleanPoint.dataPointId, noDataRevision);

  // Double dispatch before React can render disabled state must still create a
  // single command; the runtime's per-node in-flight guard is the evidence.
  page.on("dialog", (dialog) => void dialog.accept());
  let commandPosts = 0;
  page.on("request", (request) => { if (request.method() === "POST" && new URL(request.url()).pathname === "/api/command") commandPosts += 1; });
  const writeButton = page.getByRole("button", { name: /设置现场开/ });
  const writeCountBefore = simulatorWriteCount();
  await writeButton.evaluate((element) => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  await waitCommandFeedback(page, "等待设备响应");
  await eventually("only one HMI Command create", async () => commandPosts, (count) => count === 1, 10000, 50);
  await waitForCommandSimulatorWrite(writeCountBefore + 1);
  const clickedCommand = await eventually("HMI Button Command SUCCEEDED", () => latestCommandStatus(page, "设置现场开"), (value) => value === "命令执行成功", 30000, 250);
  assert.equal(clickedCommand, "命令执行成功");
  await waitPoint(booleanPoint.dataPointId, (point) => point.currentValue.quality === "GOOD" && point.currentValue.value === true, "M6 button action reflected by real BOOLEAN DataPoint");
  await page.getByRole("button", { name: /^开$/ }).waitFor({ timeout: 30000 });

  const beforeSwitch = await current(booleanPoint.dataPointId);
  assert.equal(beforeSwitch.currentValue.value, true);
  const switchWriteBefore = simulatorWriteCount();
  const switchButton = page.getByRole("button", { name: /^开$/ });
  await switchButton.click();
  await waitCommandFeedback(page, "已接收");
  const whileAccepted = await current(booleanPoint.dataPointId);
  assert.equal(whileAccepted.currentValue.value, true, "switch changed before a M5 CurrentValue update (optimistic state)");
  await page.getByRole("button", { name: /^开$/ }).waitFor();
  const switchCommand = await eventually("HMI switch Command SUCCEEDED", () => latestCommandStatus(page, "现场开关"), (value) => value === "命令执行成功", 30000, 250);
  assert.equal(switchCommand, "命令执行成功");
  await waitForCommandSimulatorWrite(switchWriteBefore + 1);
  await waitPoint(booleanPoint.dataPointId, (point) => point.currentValue.quality === "GOOD" && point.currentValue.value === false && point.currentValue.revision > beforeSwitch.currentValue.revision, "real switch state update through M4/M5");
  await page.getByRole("button", { name: /^关$/ }).waitFor({ timeout: 30000 });
  log(JSON.stringify({ scenario: "real-button-switch-and-local-inflight", status: "PASS", commandPosts, simulatorWrites: simulatorWriteCount() - writeCountBefore }));

  const runOnly = await createRunOnlyUser();
  const restrictedRuntime = await getRuntime(pageID, runOnly.token);
  assert.equal(restrictedRuntime.response.status, 200, JSON.stringify(restrictedRuntime.payload));
  assert.equal(restrictedRuntime.payload.data.canExecuteCommands, false);
  const forbidden = await requestAPI(cloudURL, runOnly.token, "/api/command", { method: "POST", body: { commandId: randomUUID(), deviceId: numberPoint.deviceId, name: "set_clock", args: { hour: 9, minute: 9, second: 1 }, ttlSeconds: 30 } });
  assert.equal(forbidden.response.status, 403, JSON.stringify(forbidden.payload));
  const restrictedPage = await openRuntimeBrowser(pageID, runOnly.user.username, "admin123");
  restrictedRuntimePage = restrictedPage;
  await restrictedPage.getByRole("button", { name: /设置现场开/ }).waitFor();
  assert.equal(await restrictedPage.getByRole("button", { name: /设置现场开/ }).isDisabled(), true, "hmi:run user without command:execute must see disabled controls");
  await restrictedPage.getByText("当前账号没有命令执行权限", { exact: true }).first().waitFor();
  const deniedNumber = await current(numberPoint.dataPointId);
  assert.equal(deniedNumber.currentValue.quality, "GOOD", "no permission must not disable read-only runtime data");
  log(JSON.stringify({ scenario: "runtime-only-and-command-execute-forbidden", status: "PASS", runtimeVisible: true, canExecuteCommands: false, commandStatus: forbidden.response.status }));

  return { runOnly };
}

function simulatorWriteCount() {
  return (simulatorProcess?.output ?? "").split("\n").filter((line) => line.includes("function=10 address=100 count=3 result=OK")).length;
}

async function waitForCommandSimulatorWrite(expected) {
  await eventually("real Modbus FC16 command write", () => simulatorWriteCount(), (count) => count >= expected, 30000, 100);
}

async function latestCommandStatus(page, buttonLabel) {
  const target = buttonLabel === "现场开关"
    ? page.getByText("现场开关", { exact: true }).first().locator("../..")
    : page.getByRole("button", { name: new RegExp(buttonLabel) });
  return target.textContent().then((value) => {
    if (value?.includes("命令执行成功")) return "命令执行成功";
    if (value?.includes("边缘设备已接收")) return "边缘设备已接收，等待执行结果";
    if (value?.includes("命令已提交")) return "命令已提交，等待设备响应";
    return value ?? "";
  }).catch(() => "");
}

async function restartCloudAndVerify(pageID, versionID, page) {
  const before = await readProbe(page);
  const beforeSnapshotCount = before.messages.filter((message) => message.type === "snapshot").length;
  await stopProcess(cloudProcess);
  cloudProcess = undefined;
  await waitForHTTPDown(`${cloudURL}/health`);
  await startCloud();
  cloudToken = await login(cloudURL);
  const restored = await api(cloudURL, cloudToken, `/api/hmi/page/${encodeURIComponent(pageID)}/runtime`);
  assert.equal(restored.version.versionId, versionID);
  await page.getByText("实时数据已连接", { exact: true }).waitFor({ timeout: 45000 });
  await page.waitForFunction((count) => window.__m8Probe.messages.filter((message) => message.type === "snapshot").length > count, beforeSnapshotCount, { timeout: 45000 });
  const number = await page.getByText("Number Current", { exact: true }).first().locator("..").textContent();
  assert.ok(number && !number.includes("--"), "published runtime did not recover the persisted CurrentValue after Cloud restart");
  assert.equal(await page.getByText("published-runtime-label", { exact: true }).count(), 1);
  assert.equal(await page.getByText("draft-only-label", { exact: true }).count(), 0);
  log(JSON.stringify({ scenario: "cloud-restart-published-runtime-recovery", status: "PASS", pageId: pageID, versionId: versionID }));
}

async function runAcceptance() {
  tempRoot = await mkdtemp(path.join(os.tmpdir(), `m8-hmi-${process.pid}-`));
  await startInfrastructure();
  await buildAndMigrate();
  await startSimulator();
  await startCloud();
  cloudToken = await login(cloudURL);
  await startCollector();
  collectorToken = await login(collectorURL);
  await configureCollector();
  const fixture = await createCollectorFixture();
  const device = await findCloudDevice();
  assert.ok(device.deviceId);
  const numberPoint = await createPoint(device.deviceId, "NUMBER", numberPointKey);
  const booleanPoint = await createPoint(device.deviceId, "BOOLEAN", booleanPointKey);
  const firstNumber = await waitGood(numberPoint.dataPointId);
  const firstBoolean = await waitGood(booleanPoint.dataPointId);
  assert.equal(typeof firstNumber.currentValue.value, "number");
  assert.equal(typeof firstBoolean.currentValue.value, "boolean");
  log(JSON.stringify({ scenario: "real-modbus-collector-m4-currentvalue", status: "PASS", deviceId: device.deviceId, number: firstNumber.currentValue, boolean: firstBoolean.currentValue }));

  const hmi = await assertLifecycle(device.deviceId, numberPoint, booleanPoint);
  const missing = await createPage(`M8 Missing publish ${runID}`, { schema: "hmi-page/v1", canvas: { width: 1280, height: 720 }, nodes: [] });
  const noPublishRuntime = await getRuntime(missing.pageId);
  assert.equal(noPublishRuntime.response.status, 404);

  if (!hasCommand("npm", ["--version"])) throw new Error("npm is required for the real browser acceptance");
  chromium = loadPlaywrightChromium();
  const executablePath = process.env.M8_PLAYWRIGHT_EXECUTABLE_PATH ?? chromium.executablePath();
  await access(executablePath);
  webProcess = startProcess("npm", ["run", "dev", "--", "--host", "127.0.0.1", "--port", String(webPort)], webDir, { VITE_API_BASE_URL: cloudURL });
  await waitForHTTP(`${webURL}/login`, webProcess);
  browser = await launchBrowser();
  runtimePage = await openRuntimeBrowser(hmi.pageID);
  await assertRuntimeQualityAndControls(runtimePage, hmi.pageID, numberPoint, booleanPoint, fixture);
  await restartCloudAndVerify(hmi.pageID, hmi.versionID, runtimePage);

  const summary = {
    status: "passed",
    scenarios: [
      "draft-published-isolation", "draft-revision-conflict-409", "publish-idempotency",
      "publish-rejects-type-mismatch", "real-m4-m5-number-boolean-runtime", "bad-no-data-quality",
      "websocket-disconnect-and-snapshot-reconnect", "single-deduplicated-realtime-subscription",
      "real-button-switch-m6-modbus-control", "command-execute-permission", "cloud-restart-published-recovery",
      "browser-websocket-uses-cloud-endpoint",
    ],
    database: "real-postgresql", broker: "eclipse-mosquitto:2", collector: "real-edge-collector", simulator: "real-modbus-simulator",
    pageId: hmi.pageID, publishedVersionId: hmi.versionID, dataPoints: 2,
  };
  console.log(JSON.stringify(summary));
}

function diagnostics() {
  const records = [];
  for (const [name, child] of [["cloud", cloudProcess], ["collector", collectorProcess], ["simulator", simulatorProcess], ["web", webProcess]]) {
    if (child) records.push(`${name}:\n${redact(child.output)}`);
  }
  return records.join("\n");
}

async function printComposeDiagnostics() {
  if (!composeStarted) return;
  try { process.stderr.write(`[m8-hmi] compose diagnostics:\n${redact(await compose("logs", "--no-color", "--tail", "200", "broker", "postgres"))}\n`); }
  catch (error) { process.stderr.write(`[m8-hmi] compose diagnostics unavailable: ${redact(error.message)}\n`); }
}

async function cleanup() {
  if (!cleanupEnabled) {
    log(`cleanup disabled; only owned project ${composeProject} and its child processes were left for diagnosis`);
    return;
  }
  await restrictedRuntimePage?.close().catch(() => {});
  await runtimePage?.close().catch(() => {});
  await browser?.close().catch(() => {});
  await stopProcess(webProcess);
  await stopProcess(simulatorProcess);
  await stopProcess(collectorProcess);
  await stopProcess(cloudProcess);
  for (const child of [...children]) await stopProcess(child);
  if (composeStarted) {
    try { await compose("down", "--remove-orphans", "--volumes"); }
    catch (error) { log(`owned Compose cleanup failed: ${redact(error.message)}`); }
  }
  if (aliasCreated) await rm(sourceAlias, { force: true });
  if (tempRoot) await rm(tempRoot, { recursive: true, force: true });
}

async function dependencyCheck() {
  const missing = [];
  if (!hasCommand("docker", ["info"])) missing.push("docker daemon");
  if (!hasCommand("go", ["version"])) missing.push("go");
  if (!hasCommand("uv", ["--version"])) missing.push("uv");
  for (const [label, candidate] of [["Cloud server", serverDir], ["real Collector API", collectorDir], ["real Modbus simulator", simulatorDir], ["Collector Compose fixture", composeFile]]) {
    try { await access(candidate); } catch { missing.push(label); }
  }
  try {
    const browserType = loadPlaywrightChromium();
    await access(process.env.M8_PLAYWRIGHT_EXECUTABLE_PATH ?? browserType.executablePath());
  } catch { missing.push("Playwright/Chromium"); }
  return missing;
}

const missing = await dependencyCheck();
if (missing.length > 0) {
  const message = `SKIP M8 HMI acceptance: missing ${missing.join(", ")}`;
  if (required) { process.stderr.write(`${message}\n`); process.exitCode = 1; }
  else console.log(message);
} else {
  try { await runAcceptance(); }
  catch (error) {
    process.stderr.write(`[m8-hmi] FAIL: ${redact(error.stack ?? error)}\n`);
    const output = diagnostics();
    if (output) process.stderr.write(`[m8-hmi] process diagnostics:\n${output}\n`);
    await printComposeDiagnostics();
    process.exitCode = 1;
  } finally { await cleanup(); }
}
