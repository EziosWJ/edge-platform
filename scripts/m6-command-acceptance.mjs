/*
 * M6 real Cloud Command acceptance.
 *
 * This harness owns an isolated Compose project and starts the real Cloud
 * binary, sibling Edge Collector binary, Eclipse Mosquitto, PostgreSQL and
 * the sibling Modbus PTY simulator. Normal control always starts with Cloud
 * POST /api/command. MQTT publication in this file is limited to explicitly
 * labelled result ordering/negative probes.
 *
 * Direct run:
 *   node scripts/m6-command-acceptance.mjs
 * Required CI run:
 *   M6_REQUIRED=1 task m6:command-acceptance
 */
import assert from "node:assert/strict";
import { access, mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { spawn, spawnSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { createRequire } from "node:module";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";

const repoRoot = fileURLToPath(new URL("../", import.meta.url));
const serverDir = path.join(repoRoot, "server");
const webDir = path.join(repoRoot, "web");
const requireWebPackage = createRequire(path.join(webDir, "package.json"));
const collectorRoot = path.resolve(process.env.M6_COLLECTOR_ROOT ?? path.join(repoRoot, "..", "edge-collector"));
const collectorDir = path.basename(collectorRoot) === "edge-collector-api"
  ? collectorRoot
  : path.join(collectorRoot, "edge-collector-api");
const simulatorDir = path.join(collectorRoot, "modbus-simulator");
const infraRoot = collectorRoot;
const composeFile = path.join(collectorRoot, "testdata/mqtt/docker-compose.yml");

const required = process.env.M6_REQUIRED === "1";
const cleanupEnabled = process.env.M6_CLEANUP !== "0";
const topicPrefix = process.env.M6_TOPIC_PREFIX ?? "edge";
const edgeID = process.env.M6_EDGE_ID ?? "m6-command-edge";
const sourceDeviceID = process.env.M6_SOURCE_DEVICE_ID ?? "m6-command-device";
const brokerPort = numberEnv("M6_BROKER_PORT", 18886);
const postgresPort = numberEnv("M6_POSTGRES_PORT", 15434);
const cloudPort = numberEnv("M6_CLOUD_PORT", 18206);
const collectorPort = numberEnv("M6_COLLECTOR_PORT", 18207);
const webPort = numberEnv("M6_WEB_PORT", 14206);
const composeProject = sanitize(process.env.M6_COMPOSE_PROJECT ?? `m6-command-${process.pid}`);
const cloudUser = process.env.M6_POSTGRES_USER ?? "m6_acceptance";
const cloudPassword = process.env.M6_POSTGRES_PASSWORD ?? "m6-acceptance-password";
const bootDatabase = sanitize(process.env.M6_POSTGRES_BOOT_DATABASE ?? `m6_boot_${process.pid}`);
const cloudDatabase = sanitize(process.env.M6_CLOUD_DATABASE ?? `m6_cloud_${process.pid}`);
const collectorDatabase = sanitize(process.env.M6_COLLECTOR_DATABASE ?? `m6_collector_${process.pid}`);
const cloudURL = `http://127.0.0.1:${cloudPort}`;
const collectorURL = `http://127.0.0.1:${collectorPort}`;
const webURL = `http://127.0.0.1:${webPort}`;
const sourceAlias = path.join(os.tmpdir(), `m6-command-${process.pid}-rtu`);
const commandFilter = `${topicPrefix}/${edgeID}/device/${sourceDeviceID}/command`;
const resultFilter = `${topicPrefix}/+/device/+/command-result`;

const children = new Set();
let tempRoot;
let composeStarted = false;
let cloudProcess;
let collectorProcess;
let simulatorProcess;
let webProcess;
let cloudBinary;
let cloudMigrateBinary;
let collectorBinary;
let collectorMigrateBinary;
let cloudToken;
let collectorToken;
let cloudEnvironment;
let collectorEnvironment;
let chromium;
let resultSubscriber;

function numberEnv(name, fallback) {
  const value = Number(process.env[name] ?? fallback);
  if (!Number.isInteger(value) || value < 1 || value > 65535) {
    throw new Error(`${name} must be a TCP port in [1, 65535]`);
  }
  return value;
}

function sanitize(value) {
  return (String(value).toLowerCase().replace(/[^a-z0-9_-]/g, "-").replace(/^-+|-+$/g, "") || "m6").slice(0, 48);
}

function redact(value) {
  return String(value)
    .replaceAll(cloudPassword, "<redacted-password>")
    .replaceAll(process.env.M6_JWT_SECRET ?? "m6-cloud-jwt-secret", "<redacted-jwt>")
    .replaceAll(process.env.M6_COLLECTOR_JWT_SECRET ?? "m6-collector-jwt-secret", "<redacted-jwt>");
}

function log(message) {
  process.stdout.write(`[m6-command] ${message}\n`);
}

function hasCommand(command, args = ["--version"]) {
  return spawnSync(command, args, { stdio: "ignore" }).status === 0;
}

function loadPlaywrightChromium() {
  const playwright = requireWebPackage("playwright");
  const browser = playwright.chromium ?? playwright.default?.chromium;
  if (!browser || typeof browser.executablePath !== "function") {
    throw new Error("resolved Playwright package does not expose chromium");
  }
  return browser;
}

function startProcess(command, args, cwd, environment = {}) {
  const child = spawn(command, args, {
    cwd,
    env: { ...process.env, ...environment },
    detached: true,
    stdio: ["ignore", "pipe", "pipe"],
  });
  child.output = "";
  const collect = (chunk) => { child.output = `${child.output}${chunk}`.slice(-120000); };
  child.stdout?.on("data", collect);
  child.stderr?.on("data", collect);
  children.add(child);
  return child;
}

function signalProcessGroup(child, signal) {
  if (!child?.pid) return;
  try { process.kill(-child.pid, signal); } catch { /* process group already exited */ }
  try { process.kill(child.pid, signal); } catch { /* process already exited */ }
}

async function stopProcess(child, signal = "SIGTERM") {
  if (!child) return;
  if (child.exitCode === null && child.signalCode === null) {
    const closed = new Promise((resolve) => child.once("close", resolve));
    signalProcessGroup(child, signal);
    await Promise.race([closed, delay(signal === "SIGKILL" ? 2000 : 5000)]);
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
  if (result.code !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed (${result.code ?? result.signal}):\n${redact(child.output)}`);
  }
  return child.output;
}

function composeArgs(...args) {
  return ["compose", "--project-name", composeProject, "--file", composeFile, ...args];
}

const composeEnvironment = {
  MQTT_E2E_BROKER_PORT: String(brokerPort),
  MQTT_E2E_POSTGRES_PORT: String(postgresPort),
  MQTT_E2E_POSTGRES_DB: bootDatabase,
  MQTT_E2E_POSTGRES_USER: cloudUser,
  MQTT_E2E_POSTGRES_PASSWORD: cloudPassword,
};

async function compose(...args) {
  return runCommand("docker", composeArgs(...args), infraRoot, composeEnvironment);
}

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
    } catch {
      // Process is still starting.
    }
    if (child && child.exitCode !== null) {
      throw new Error(`process exited while waiting for ${url}:\n${redact(child.output)}`);
    }
    await delay(250);
  }
  throw new Error(`endpoint did not become ready: ${url}\n${redact(child?.output ?? "")}`);
}

async function waitForHTTPDown(url, timeout = 15000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try {
      await fetch(url, { signal: AbortSignal.timeout(500) });
    } catch {
      return;
    }
    await delay(100);
  }
  throw new Error(`endpoint remained reachable: ${url}`);
}

async function eventually(label, read, predicate, timeout = 60000, interval = 250) {
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
      try {
        const value = JSON.parse(line);
        return Array.isArray(value) ? value : [value];
      } catch {
        return [];
      }
    });
    if (records.some((record) => record.Service === service &&
      (record.Health === "healthy" || String(record.Status ?? "").includes("(healthy)")))) return;
    await delay(500);
  }
  throw new Error(`Compose service did not become healthy: ${service}`);
}

function databaseURL(database) {
  return `postgres://127.0.0.1:${postgresPort}/${database}?sslmode=disable`;
}

function sqlLiteral(value) {
  return `'${String(value).replaceAll("'", "''")}'`;
}

function sqlIdentifier(value) {
  return `"${String(value).replaceAll('"', '""')}"`;
}

async function postgresQuery(database, query) {
  return compose(
    "exec", "-T", "postgres", "env", `PGPASSWORD=${cloudPassword}`, "psql",
    "-h", "127.0.0.1", "-U", cloudUser, "-d", database, "-At", "-v", "ON_ERROR_STOP=1", "-c", query,
  );
}

async function retryPostgres(label, operation, timeout = 60000, interval = 500) {
  const deadline = Date.now() + timeout;
  let lastError;
  while (Date.now() < deadline) {
    try {
      return await operation();
    } catch (error) {
      lastError = error;
      await delay(interval);
    }
  }
  throw new Error(`${label} did not become ready: ${redact(lastError?.message ?? lastError ?? "unknown PostgreSQL error")}`);
}

async function createDatabase(database) {
  const existing = (await retryPostgres(
    `PostgreSQL admin query for ${database}`,
    () => postgresQuery("postgres", `SELECT 1 FROM pg_database WHERE datname=${sqlLiteral(database)}`),
  )).trim();
  if (!existing) {
    await retryPostgres(
      `PostgreSQL create database ${database}`,
      () => compose("exec", "-T", "postgres", "env", `PGPASSWORD=${cloudPassword}`, "psql", "-h", "127.0.0.1", "-U", cloudUser, "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", `CREATE DATABASE ${sqlIdentifier(database)} OWNER ${sqlIdentifier(cloudUser)}`),
    );
  }
}

function cloudEnv() {
  return {
    APP_ENV: "dev",
    APP_DATABASE__DRIVER: "postgres",
    APP_DATABASE__URL: databaseURL(cloudDatabase),
    APP_DATABASE__USERNAME: cloudUser,
    APP_DATABASE__PASSWORD: cloudPassword,
    APP_HTTP__ADDRESS: `127.0.0.1:${cloudPort}`,
    APP_JWT__SECRET: process.env.M6_JWT_SECRET ?? "m6-cloud-jwt-secret",
    APP_FILE__STORAGE_ROOT: path.join(tempRoot, "cloud-files"),
    APP_LOG__LEVEL: "info",
    APP_LOG__FORMAT: "text",
    APP_MQTT__ENABLED: "true",
    APP_MQTT__URL: `mqtt://127.0.0.1:${brokerPort}`,
    APP_MQTT__PROTOCOL: "mqtt5",
    APP_MQTT__CLIENT_ID: `m6-cloud-${process.pid}`,
    APP_MQTT__PREFIX: topicPrefix,
    APP_MQTT__USERNAME: "",
    APP_MQTT__PASSWORD: "",
    GOCACHE: process.env.GOCACHE ?? path.join(tempRoot, "go-cache-cloud"),
  };
}

function collectorEnv() {
  return {
    APP_ENV: "dev",
    APP_DATABASE__DRIVER: "postgres",
    APP_DATABASE__URL: databaseURL(collectorDatabase),
    APP_DATABASE__USERNAME: cloudUser,
    APP_DATABASE__PASSWORD: cloudPassword,
    APP_HTTP__ADDRESS: `127.0.0.1:${collectorPort}`,
    APP_JWT__SECRET: process.env.M6_COLLECTOR_JWT_SECRET ?? "m6-collector-jwt-secret",
    APP_FILE__STORAGE_ROOT: path.join(tempRoot, "collector-files"),
    APP_LOG__LEVEL: "info",
    APP_LOG__FORMAT: "text",
    APP_ACQUISITION__SCRIPT__MAX_EXECUTION_MS: "15000",
    APP_ACQUISITION__SCRIPT__MAX_TOTAL_DELAY_MS: "8000",
    GOCACHE: process.env.GOCACHE ?? path.join(tempRoot, "go-cache-collector"),
  };
}

async function startInfrastructure() {
  await compose("up", "-d", "--remove-orphans", "postgres", "broker");
  composeStarted = true;
  await waitForPort("127.0.0.1", postgresPort);
  await waitForPort("127.0.0.1", brokerPort);
  await waitComposeHealthy("postgres");
  await waitComposeHealthy("broker");
  await createDatabase(cloudDatabase);
  await createDatabase(collectorDatabase);
  log(`isolated PostgreSQL/Mosquitto ready (project=${composeProject})`);
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
  const configRoot = path.join(tempRoot, "modbus-simulator");
  const devicesRoot = path.join(configRoot, "devices");
  await mkdir(devicesRoot, { recursive: true });
  await writeFile(
    path.join(devicesRoot, "time_registers_03.yaml"),
    await readFile(path.join(simulatorDir, "config/devices/time_registers_03.yaml"), "utf8"),
  );
  const configPath = path.join(configRoot, "m6-command.yaml");
  await writeFile(configPath, `logging:\n  level: INFO\n  hex: false\n\nchannels:\n  - name: m6-command-rtu\n    protocol: rtu\n    alias: ${sourceAlias}\n    baudrate: 9600\n    bytesize: 8\n    parity: N\n    stopbits: 1\n    devices:\n      - devices/time_registers_03.yaml\n`);
  return configPath;
}

async function startSimulator() {
  const configPath = await writeSimulatorConfig();
  simulatorProcess = startProcess("uv", ["run", "modbus-simulator", "--config", configPath], simulatorDir, {
    UV_CACHE_DIR: path.join(tempRoot, "uv-cache"),
  });
  await waitForOutput(simulatorProcess, "Modbus Simulator Started", 60000);
  await waitForPath(sourceAlias, 30000);
  log("real Modbus simulator PTY ready");
}

async function waitForOutput(child, expected, timeout = 30000) {
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
    try { await access(file); return; } catch { await delay(100); }
  }
  throw new Error(`path did not become ready: ${file}`);
}

async function startCloud() {
  cloudProcess = startProcess(cloudBinary, [], serverDir, cloudEnvironment);
  await waitForHTTP(`${cloudURL}/health`, cloudProcess);
  log("real Cloud healthy");
}

async function startCollector() {
  collectorProcess = startProcess(collectorBinary, [], collectorDir, collectorEnvironment);
  await waitForHTTP(`${collectorURL}/health`, collectorProcess);
  log("real Edge Collector healthy");
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

async function login(baseURL, username, password) {
  const result = await requestJSON(baseURL, "/api/auth/login", "", { method: "POST", body: { username, password } });
  assert.equal(result.response.status, 200, `login failed at ${baseURL}: ${JSON.stringify(result.payload)}`);
  assert.equal(result.payload.code, 200, `login failed at ${baseURL}: ${JSON.stringify(result.payload)}`);
  assert.ok(result.payload.data?.tokenValue, "login returned no token");
  return result.payload.data.tokenValue;
}

async function api(baseURL, token, pathname, options = {}, statuses = [200]) {
  const result = await requestJSON(baseURL, pathname, token, options);
  assert.ok(statuses.includes(result.response.status), `${options.method ?? "GET"} ${pathname}: ${JSON.stringify(result.payload)}`);
  assert.equal(result.payload.code, 200, `${options.method ?? "GET"} ${pathname}: ${JSON.stringify(result.payload)}`);
  return result.payload.data;
}

async function configureCollector() {
  await api(collectorURL, collectorToken, "/api/v1/mqtt/config", {
    method: "PUT",
    body: {
      enabled: true,
      edgeId: edgeID,
      brokerUrl: `mqtt://127.0.0.1:${brokerPort}`,
      protocolVersion: "MQTT_5",
      clientId: `m6-collector-${process.pid}`,
      username: "",
      passwordAction: "clear",
      password: "",
      tlsEnabled: false,
      caCertificate: "",
      clientCertificate: "",
      clientPrivateKeyAction: "clear",
      clientPrivateKey: "",
      keepAliveSeconds: 5,
      connectTimeoutMs: 3000,
      reconnectMinMs: 200,
      reconnectMaxMs: 1000,
      topicPrefix,
      rawPublishIntervalMs: 500,
      outboxMaxRows: 100,
      outboxMaxBytes: 8 * 1024 * 1024,
      outboxRetentionDays: 7,
      commandJournalRetentionDays: 7,
      commandJournalMaxRows: 100,
      commandQueueCapacity: 8,
      commandPollFairness: 1,
    },
  });
  await eventually(
    "Collector MQTT connected",
    () => api(collectorURL, collectorToken, "/api/v1/mqtt/state"),
    (state) => state.state === "CONNECTED" && state.connected === true,
    45000,
  );
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
    if args.get("holdForRestart", False):
        for _ in range(6):
            ctx.delay(1000)
    else:
        ctx.delay(500)
    ctx.write_registers(100, [args["hour"], args["minute"], args["second"]])
    return {"written": [args["hour"], args["minute"], args["second"]]}
`;
}

async function createCollectorFixture() {
  const script = await api(collectorURL, collectorToken, "/api/v1/acquisition/scripts", {
    method: "POST",
    body: { name: `M6 command acceptance ${process.pid}`, description: "real M6 command fixture", draftSource: commandSource() },
  });
  await api(collectorURL, collectorToken, `/api/v1/acquisition/scripts/${script.id}/validate`, { method: "POST" });
  await api(collectorURL, collectorToken, `/api/v1/acquisition/scripts/${script.id}/publish`, { method: "POST" });
  const channel = await api(collectorURL, collectorToken, "/api/v1/acquisition/channels", {
    method: "POST",
    body: {
      name: `M6 command RTU ${process.pid}`,
      protocol: "MODBUS_RTU",
      serialConfig: { port: sourceAlias, baudRate: 9600, dataBits: 8, stopBits: 1, parity: "N" },
      timeoutMs: 500,
      interRequestDelayMs: 5,
      enabled: 1,
    },
  });
  const device = await api(collectorURL, collectorToken, "/api/v1/acquisition/devices", {
    method: "POST",
    body: {
      externalId: sourceDeviceID,
      name: "M6 command simulator",
      deviceType: "FEED_PROTECTOR",
      channelId: channel.id,
      unitId: 3,
      pollIntervalMs: 250,
      failureThreshold: 3,
      enabled: 1,
      registerBlocks: [{ name: "rtc-raw", functionCode: 3, startAddress: 100, quantity: 3, sortOrder: 0 }],
    },
  });
  await api(collectorURL, collectorToken, `/api/v1/acquisition/devices/${device.id}/script`, {
    method: "PUT",
    body: { scriptId: script.id },
  });
  await eventually(
    "real Collector device ONLINE",
    () => api(collectorURL, collectorToken, `/api/v1/acquisition/states/${device.id}`),
    (state) => state.status === "ONLINE" && state.registerBlocks?.every((block) => block.valid),
    60000,
  );
  return { script, channel, device };
}

async function findCloudDevice() {
  return eventually(
    "Cloud Device discovery",
    async () => {
      const page = await api(cloudURL, cloudToken, `/api/device/page?page=1&pageSize=100&edgeId=${encodeURIComponent(edgeID)}&sourceDeviceId=${encodeURIComponent(sourceDeviceID)}`);
      return page.records?.find((record) => record.edgeId === edgeID && record.sourceDeviceId === sourceDeviceID);
    },
    (device) => Boolean(device),
    60000,
  );
}

async function waitCommand(commandID, predicate, label = `Command ${commandID}`) {
  return eventually(label, async () => {
    const result = await requestJSON(cloudURL, `/api/command/${encodeURIComponent(commandID)}`, cloudToken);
    return result.response.status === 200 && result.payload.code === 200 ? result.payload.data : null;
  }, predicate, 90000, 250);
}

async function readCollectorDiagnostic(pathname) {
  try {
    const result = await requestJSON(collectorURL, pathname, collectorToken);
    return { status: result.response.status, payload: result.payload };
  } catch (error) {
    return { error: redact(error.message ?? error) };
  }
}

async function logCollectorCommandDiagnostics(commandID, collectorDeviceID) {
  const [command, mqttState, deviceState, scriptState] = await Promise.all([
    readCollectorDiagnostic(`/api/v1/mqtt/commands/${encodeURIComponent(commandID)}`),
    readCollectorDiagnostic("/api/v1/mqtt/state"),
    readCollectorDiagnostic(`/api/v1/acquisition/states/${collectorDeviceID}`),
    readCollectorDiagnostic(`/api/v1/acquisition/script-states/${collectorDeviceID}`),
  ]);
  log(`diagnostic collector command=${JSON.stringify(command)} mqtt=${JSON.stringify(mqttState)} device=${JSON.stringify(deviceState)} script=${JSON.stringify(scriptState)}`);
}

async function postCommand(commandID, deviceID, args, ttlSeconds = 30, name = "set_clock") {
  const body = { commandId: commandID, deviceId: deviceID, name, args, ttlSeconds };
  const result = await requestJSON(cloudURL, "/api/command", cloudToken, { method: "POST", body });
  assert.equal(result.response.status, 202, `Command create must return HTTP 202: ${JSON.stringify(result.payload)}`);
  assert.equal(result.payload.code, 200, `Command create failed: ${JSON.stringify(result.payload)}`);
  return { body, data: result.payload.data };
}

class MQTTSubscriber {
  constructor(child) {
    this.child = child;
    this.messages = [];
    this.buffer = "";
    child.stdout.on("data", (chunk) => {
      this.buffer += chunk.toString();
      const lines = this.buffer.split("\n");
      this.buffer = lines.pop() ?? "";
      for (const line of lines) this.readLine(line.replace(/\r$/, ""));
    });
  }

  readLine(line) {
    if (!line) return;
    const first = line.indexOf("|");
    const second = line.indexOf("|", first + 1);
    const third = line.indexOf("|", second + 1);
    if (first < 0 || second < 0 || third < 0) return;
    const rawPayload = line.slice(third + 1);
    try {
      this.messages.push({
        topic: line.slice(0, first),
        qos: Number(line.slice(first + 1, second)),
        retained: Number(line.slice(second + 1, third)) === 1,
        rawPayload,
        payload: JSON.parse(rawPayload),
        receivedAt: Date.now(),
      });
    } catch {
      // Ignore broker startup noise and non-JSON diagnostics.
    }
  }

  async waitFor(predicate, fromIndex = 0, timeout = 60000) {
    const deadline = Date.now() + timeout;
    while (Date.now() < deadline) {
      for (let index = fromIndex; index < this.messages.length; index += 1) {
        if (predicate(this.messages[index])) return { message: this.messages[index], index };
      }
      if (this.child.exitCode !== null) throw new Error(`MQTT subscriber exited:\n${redact(this.child.output)}`);
      await delay(100);
    }
    throw new Error(`MQTT message did not arrive; received ${this.messages.length}`);
  }
}

async function startMQTTSubscriber(filter) {
  const subscriberProcess = startProcess("docker", composeArgs("exec", "-T", "broker", "mosquitto_sub", "-V", "mqttv5", "-h", "127.0.0.1", "-p", "1883", "-t", filter, "-q", "1", "-F", "%t|%q|%r|%p"), infraRoot, composeEnvironment);
  const subscriber = new MQTTSubscriber(subscriberProcess);
  await delay(500);
  if (subscriberProcess.exitCode !== null) throw new Error(`MQTT subscriber failed:\n${redact(subscriberProcess.output)}`);
  return { process: subscriberProcess, subscriber };
}

async function publishMQTT(topic, payload) {
  await runCommand("docker", composeArgs("exec", "-T", "broker", "mosquitto_pub", "-V", "mqttv5", "-h", "127.0.0.1", "-p", "1883", "-t", topic, "-q", "1", "-m", JSON.stringify(payload)), infraRoot, composeEnvironment);
}

function commandResultPayload(commandID, status, overrides = {}) {
  const receivedAt = overrides.receivedAt ?? new Date(Date.now() - 1000).toISOString();
  const startedAt = overrides.startedAt ?? new Date(Date.parse(receivedAt) + 100).toISOString();
  const completedAt = overrides.completedAt ?? new Date(Date.parse(receivedAt) + 200).toISOString();
  return {
    schema: "device-command-result/v1",
    messageId: `m6-probe-${randomUUID()}`,
    edgeId: overrides.edgeId ?? edgeID,
    deviceId: overrides.deviceId ?? sourceDeviceID,
    timestamp: overrides.timestamp ?? new Date().toISOString(),
    data: {
      commandId: commandID,
      name: overrides.name ?? "set_clock",
      status,
      receivedAt,
      startedAt: status === "ACCEPTED" ? null : startedAt,
      completedAt: status === "ACCEPTED" ? null : completedAt,
      result: overrides.result ?? (status === "ACCEPTED" ? null : { probe: true }),
      error: overrides.error ?? null,
    },
  };
}

async function waitResult(commandID, status, fromIndex = 0) {
  const result = await resultSubscriber.subscriber.waitFor(
    (message) => message.topic === `${topicPrefix}/${edgeID}/device/${sourceDeviceID}/command-result`
      && message.payload?.schema === "device-command-result/v1"
      && message.payload.data?.commandId === commandID
      && message.payload.data?.status === status,
    fromIndex,
    90000,
  );
  assert.equal(result.message.qos, 1, "command-result must be QoS1");
  assert.equal(result.message.retained, false, "command-result must not be retained");
  return result;
}

function simulatorWriteCount() {
  return (simulatorProcess?.output ?? "").split("\n").filter((line) =>
    line.includes("function=10 address=100 count=3 result=OK")).length;
}

function simulatorPollCount() {
  return (simulatorProcess?.output ?? "").split("\n").filter((line) =>
    line.includes("function=03 address=100 count=3 result=OK")).length;
}

async function metrics() {
  const response = await fetch(`${cloudURL}/metrics`, { signal: AbortSignal.timeout(10000) });
  assert.equal(response.status, 200);
  return response.text();
}

async function assertMetricClassification(classification) {
  await eventually(`metric classification ${classification}`, metrics, (text) =>
    new RegExp(`command_result_projection_events_total\\{classification="${classification}"\\} [1-9]`).test(text), 30000, 250);
}

async function assertMQTTIngressRejection(reason) {
  await eventually(`MQTT ingress rejection ${reason}`, metrics, (text) =>
    new RegExp(`mqtt_ingress_rejections_total\\{reason="${reason}"\\} [1-9]`).test(text), 30000, 250);
}

async function commandFacts(commandID) {
  const value = (await postgresQuery(cloudDatabase, `SELECT (SELECT count(*) FROM command WHERE command_id=${sqlLiteral(commandID)}) || '|' || (SELECT count(*) FROM command_delivery WHERE command_id=${sqlLiteral(commandID)}) || '|' || (SELECT count(*) FROM sys_oper_log WHERE module_name='command' AND operation_type='command.create' AND operator_id=1)`)).trim();
  const [commands, deliveries, audits] = value.split("|").map(Number);
  return { commands, deliveries, audits };
}

async function deliveryPayloadDigest(commandID) {
  const value = (await postgresQuery(cloudDatabase, `SELECT encode(payload, 'base64') FROM command_delivery WHERE command_id=${sqlLiteral(commandID)}`)).trim();
  assert.ok(value, `delivery payload for ${commandID} is missing`);
  return createHash("sha256").update(Buffer.from(value, "base64")).digest("hex");
}

async function restartCloud() {
  await stopProcess(cloudProcess, "SIGTERM");
  cloudProcess = undefined;
  await waitForHTTPDown(`${cloudURL}/health`);
  cloudProcess = startProcess(cloudBinary, [], serverDir, cloudEnvironment);
  await waitForHTTP(`${cloudURL}/health`, cloudProcess);
  cloudToken = await login(cloudURL, "admin", "admin123");
}

async function restartCollector() {
  await stopProcess(collectorProcess, "SIGTERM");
  collectorProcess = undefined;
  await waitForHTTPDown(`${collectorURL}/health`);
  collectorProcess = startProcess(collectorBinary, [], collectorDir, collectorEnvironment);
  await waitForHTTP(`${collectorURL}/health`, collectorProcess);
  collectorToken = await login(collectorURL, "admin", "admin123");
  await configureCollector();
}

async function stopBroker() {
  await compose("kill", "-s", "SIGKILL", "broker");
  await stopResultSubscriber();
}

async function startBroker() {
  await compose("up", "-d", "broker");
  await waitForPort("127.0.0.1", brokerPort);
  await waitComposeHealthy("broker");
  resultSubscriber = await startMQTTSubscriber(resultFilter);
}

async function stopResultSubscriber() {
  if (!resultSubscriber?.process) {
    resultSubscriber = undefined;
    return;
  }
  await stopProcess(resultSubscriber.process, "SIGTERM");
  resultSubscriber = undefined;
}

async function createNoRoleUser() {
  const username = `m6-no-role-${process.pid}`;
  await api(cloudURL, cloudToken, "/api/system/user", {
    method: "POST",
    body: { username, nickname: "M6 no role", gender: "UNSPECIFIED", status: 1 },
  });
  return login(cloudURL, username, "admin123");
}

async function runUIAcceptance(deviceID) {
  chromium = loadPlaywrightChromium();
  const executablePath = process.env.M6_PLAYWRIGHT_EXECUTABLE_PATH ?? chromium.executablePath();
  await access(executablePath);
  webProcess = startProcess("npm", ["run", "dev", "--", "--host", "127.0.0.1", "--port", String(webPort)], webDir, {
    VITE_API_BASE_URL: cloudURL,
  });
  await waitForHTTP(`${webURL}/login`, webProcess);
  const browser = await chromium.launch({ headless: true, executablePath });
  try {
    const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
    await page.goto(`${webURL}/login`, { waitUntil: "networkidle" });
    await page.getByLabel("用户名").fill("admin");
    await page.locator("#password").fill("admin123");
    await page.getByRole("button", { name: "登录" }).click();
    await page.waitForURL(/\/dashboard/);
    await page.goto(`${webURL}/device`, { waitUntil: "networkidle" });
    await page.getByText(deviceID, { exact: true }).waitFor();
    await page.getByRole("button", { name: /执行命令/ }).first().click();
    await page.getByPlaceholder("例如 close").fill("set_clock");
    await page.getByLabel("JSON args").fill('{"hour":16,"minute":26,"second":40}');
    await page.locator('input[type="number"]').fill("30");
    await page.getByRole("button", { name: "提交执行" }).click();
    await page.getByText("Command 详情", { exact: true }).waitFor();
    await page.getByText("已成功", { exact: true }).waitFor({ timeout: 90000 });
    log("PASS UI login -> Device execute -> Command detail observed PENDING to terminal result");
  } finally {
    await browser.close();
  }
}

async function runAcceptance() {
  tempRoot = await mkdtemp(path.join(os.tmpdir(), "m6-command-acceptance-"));
  await startInfrastructure();
  await buildAndMigrate();
  await startSimulator();
  await startCloud();
  cloudToken = await login(cloudURL, "admin", "admin123");
  await startCollector();
  collectorToken = await login(collectorURL, "admin", "admin123");
  await configureCollector();
  const collectorFixture = await createCollectorFixture();
  const device = await findCloudDevice();
  assert.ok(device.deviceId);
  resultSubscriber = await startMQTTSubscriber(resultFilter);
  log(`real Cloud Device discovered: ${device.deviceId}`);

  const baselineWrites = simulatorWriteCount();
  const firstCommandID = randomUUID();
  const firstStart = resultSubscriber.subscriber.messages.length;
  const first = await postCommand(firstCommandID, device.deviceId, { hour: 10, minute: 20, second: 30 });
  assert.equal(first.data.status, "PENDING");
  let final;
  try {
    final = await waitResult(firstCommandID, "SUCCEEDED", firstStart);
  } catch (error) {
    await logCollectorCommandDiagnostics(firstCommandID, collectorFixture.device.id);
    throw error;
  }
  const detail = await waitCommand(firstCommandID, (value) => value?.status === "SUCCEEDED", "normal Cloud command SUCCEEDED");
  assert.deepEqual(detail.args, { hour: 10, minute: 20, second: 30 });
  assert.equal(detail.status, "SUCCEEDED");
  assert.ok(detail.edgeReceivedAt && detail.startedAt && detail.completedAt && detail.resultReceivedAt);
  assert.equal(Date.parse(detail.edgeReceivedAt), Date.parse(final.message.payload.data.receivedAt));
  assert.ok(Date.parse(final.message.payload.timestamp) >= Date.parse(final.message.payload.data.receivedAt), "Edge envelope timestamp must not precede Edge receivedAt");
  assert.ok(Date.parse(detail.resultReceivedAt) >= Date.parse(final.message.payload.timestamp), "Cloud resultReceivedAt must be observed after the Edge envelope");
  assert.notEqual(detail.resultReceivedAt, detail.edgeReceivedAt, "Cloud resultReceivedAt must remain separate from Edge receivedAt");
  await eventually("one real FC16 write", () => simulatorWriteCount(), (count) => count === baselineWrites + 1, 30000);
  assert.equal(simulatorWriteCount(), baselineWrites + 1);
  const firstFacts = await commandFacts(firstCommandID);
  assert.deepEqual(firstFacts, { commands: 1, deliveries: 0, audits: 1 });
  log(JSON.stringify({ scenario: "01-real-command-success", status: "PASS", commandId: firstCommandID, edgeTimestamp: final.message.payload.timestamp, edgeReceivedAt: detail.edgeReceivedAt, cloudResultReceivedAt: detail.resultReceivedAt, fc16Writes: simulatorWriteCount() - baselineWrites }));

  const retry = await postCommand(firstCommandID, device.deviceId, { hour: 10, minute: 20, second: 30 });
  assert.equal(retry.data.commandId, firstCommandID);
  const conflict = await requestJSON(cloudURL, "/api/command", cloudToken, { method: "POST", body: { commandId: firstCommandID, deviceId: device.deviceId, name: "set_clock", args: { hour: 10, minute: 20, second: 31 }, ttlSeconds: 30 } });
  assert.equal(conflict.response.status, 409);
  assert.equal(conflict.payload.code, 409);
  for (const variant of [
    { name: "other_command", args: { hour: 10, minute: 20, second: 30 }, ttlSeconds: 30 },
    { name: "set_clock", args: { hour: 10, minute: 20, second: 30 }, ttlSeconds: 29 },
  ]) {
    const variantConflict = await requestJSON(cloudURL, "/api/command", cloudToken, {
      method: "POST",
      body: { commandId: firstCommandID, deviceId: device.deviceId, ...variant },
    });
    assert.equal(variantConflict.response.status, 409, `same commandId variant must conflict: ${JSON.stringify(variant)}`);
  }
  assert.equal(simulatorWriteCount(), baselineWrites + 1);
  assert.deepEqual(await commandFacts(firstCommandID), { commands: 1, deliveries: 0, audits: 1 });
  log(JSON.stringify({ scenario: "02-http-idempotency-and-conflict", status: "PASS", commandId: firstCommandID, modbusWrites: simulatorWriteCount() - baselineWrites }));

  const noRoleToken = await createNoRoleUser();
  const unauthorized = await requestJSON(cloudURL, "/api/command", noRoleToken, { method: "POST", body: { commandId: randomUUID(), deviceId: device.deviceId, name: "set_clock", args: { hour: 1, minute: 2, second: 3 }, ttlSeconds: 30 } });
  assert.equal(unauthorized.response.status, 403);
  log(JSON.stringify({ scenario: "03-server-side-unauthorized", status: "PASS", httpStatus: unauthorized.response.status }));

  await stopBroker();
  const brokerRecoveryID = randomUUID();
  const brokerRecovery = await postCommand(brokerRecoveryID, device.deviceId, { hour: 11, minute: 21, second: 31 });
  assert.equal(brokerRecovery.data.status, "PENDING");
  await delay(1500);
  const pendingDuringBrokerPause = await waitCommand(brokerRecoveryID, (value) => value?.status === "PENDING", "pending while broker is paused");
  assert.equal(pendingDuringBrokerPause.status, "PENDING");
  await startBroker();
  const recoveryFinal = await waitResult(brokerRecoveryID, "SUCCEEDED");
  await waitCommand(brokerRecoveryID, (value) => value?.status === "SUCCEEDED");
  await eventually("broker recovery executes one FC16", () => simulatorWriteCount(), (count) => count === baselineWrites + 2, 60000);
  log(JSON.stringify({ scenario: "04-broker-pause-recovery", status: "PASS", commandId: brokerRecoveryID, writes: simulatorWriteCount() - baselineWrites, resultTimestamp: recoveryFinal.message.payload.timestamp }));

  await stopBroker();
  const restartID = randomUUID();
  await postCommand(restartID, device.deviceId, { hour: 12, minute: 22, second: 32 });
  const frozenDigest = await deliveryPayloadDigest(restartID);
  await stopProcess(cloudProcess, "SIGKILL");
  cloudProcess = undefined;
  await waitForHTTPDown(`${cloudURL}/health`);
  await startBroker();
  const commandSubscriber = await startMQTTSubscriber(commandFilter);
  cloudProcess = startProcess(cloudBinary, [], serverDir, cloudEnvironment);
  await waitForHTTP(`${cloudURL}/health`, cloudProcess);
  cloudToken = await login(cloudURL, "admin", "admin123");
  const publishedCommand = await commandSubscriber.subscriber.waitFor((message) => message.payload?.commandId === restartID, 0, 60000);
  const publishedDigest = createHash("sha256").update(publishedCommand.message.rawPayload).digest("hex");
  assert.equal(publishedDigest, frozenDigest, "Cloud restart changed frozen command payload bytes");
  await waitCommand(restartID, (value) => value?.status === "SUCCEEDED", "Cloud restart command recovery");
  await eventually("Cloud restart executes one FC16", () => simulatorWriteCount(), (count) => count === baselineWrites + 3, 60000);
  await stopProcess(commandSubscriber.process, "SIGTERM");
  log(JSON.stringify({ scenario: "05-cloud-restart-frozen-payload", status: "PASS", commandId: restartID, payloadSha256: frozenDigest, writes: simulatorWriteCount() - baselineWrites }));

  await stopProcess(collectorProcess, "SIGTERM");
  collectorProcess = undefined;
  await waitForHTTPDown(`${collectorURL}/health`);
  const orderingID = randomUUID();
  await postCommand(orderingID, device.deviceId, { hour: 13, minute: 23, second: 33 });
  const edgeReceivedAt = new Date(Date.now() - 3000).toISOString();
  const finalProbe = commandResultPayload(orderingID, "SUCCEEDED", { receivedAt: edgeReceivedAt, result: { source: "final-before-accepted" } });
  log(`NEGATIVE/ORDER PROBE direct MQTT FINAL-before-ACCEPTED commandId=${orderingID}`);
  await publishMQTT(commandFilter.replace("/command", "/command-result"), finalProbe);
  await waitCommand(orderingID, (value) => value?.status === "SUCCEEDED", "FINAL-before-ACCEPTED projection");
  const firstProjected = await api(cloudURL, cloudToken, `/api/command/${orderingID}`);
  await publishMQTT(commandFilter.replace("/command", "/command-result"), { ...finalProbe, messageId: `m6-duplicate-${randomUUID()}`, timestamp: new Date().toISOString() });
  await publishMQTT(commandFilter.replace("/command", "/command-result"), commandResultPayload(orderingID, "ACCEPTED", { receivedAt: edgeReceivedAt }));
  await delay(1000);
  const afterLateAccepted = await api(cloudURL, cloudToken, `/api/command/${orderingID}`);
  assert.equal(afterLateAccepted.status, "SUCCEEDED");
  assert.equal(afterLateAccepted.resultReceivedAt, firstProjected.resultReceivedAt);
  const conflictProbe = commandResultPayload(orderingID, "SUCCEEDED", { receivedAt: edgeReceivedAt, result: { source: "conflict" } });
  await publishMQTT(commandFilter.replace("/command", "/command-result"), conflictProbe);
  await delay(1000);
  const afterConflict = await api(cloudURL, cloudToken, `/api/command/${orderingID}`);
  assert.deepEqual(afterConflict.result, firstProjected.result);
  assert.equal(afterConflict.resultReceivedAt, firstProjected.resultReceivedAt);
  await assertMetricClassification("duplicate");
  await assertMetricClassification("conflict");
  log(JSON.stringify({ scenario: "06-final-before-accepted-first-terminal-wins", status: "PASS", commandId: orderingID, resultReceivedAt: firstProjected.resultReceivedAt }));

  const unknownID = randomUUID();
  const beforeUnknown = await commandFacts(unknownID);
  log(`NEGATIVE PROBE unknown commandId direct MQTT commandId=${unknownID}`);
  await publishMQTT(commandFilter.replace("/command", "/command-result"), commandResultPayload(unknownID, "SUCCEEDED"));
  await assertMetricClassification("unknown_command");
  assert.deepEqual(await commandFacts(unknownID), { commands: 0, deliveries: 0, audits: beforeUnknown.audits });
  const beforeNegative = await api(cloudURL, cloudToken, `/api/command/${firstCommandID}`);
  log("NEGATIVE PROBE route mismatch direct MQTT");
  await publishMQTT(`${topicPrefix}/wrong-edge/device/${sourceDeviceID}/command-result`, commandResultPayload(firstCommandID, "SUCCEEDED", { edgeId: "other-edge", result: beforeNegative.result }));
  // The topic/envelope identity mismatch is rejected by the MQTT parser before
  // command-result projection. Assert the parser's bounded metric, not the
  // downstream projection metric.
  await assertMQTTIngressRejection("identity_mismatch");
  log("NEGATIVE PROBE name mismatch direct MQTT");
  await publishMQTT(`${topicPrefix}/${edgeID}/device/${sourceDeviceID}/command-result`, commandResultPayload(firstCommandID, "SUCCEEDED", { name: "wrong-name", result: beforeNegative.result }));
  await assertMetricClassification("name");
  const afterNegative = await api(cloudURL, cloudToken, `/api/command/${firstCommandID}`);
  assert.equal(afterNegative.status, beforeNegative.status);
  assert.equal(afterNegative.edgeId, beforeNegative.edgeId);
  assert.equal(afterNegative.sourceDeviceId, beforeNegative.sourceDeviceId);
  assert.equal(afterNegative.name, beforeNegative.name);
  assert.deepEqual(afterNegative.result, beforeNegative.result);
  await waitForHTTP(`${cloudURL}/health`, cloudProcess);
  log(JSON.stringify({ scenario: "07-negative-result-probes", status: "PASS", unknownCommandId: unknownID, cloudHealthy: true }));

  const ttlID = randomUUID();
  await postCommand(ttlID, device.deviceId, { hour: 14, minute: 24, second: 34 }, 2);
  const ttlPending = await eventually("no-result delivery expiry", async () => api(cloudURL, cloudToken, `/api/command/${ttlID}`), (value) => value?.status === "PENDING" && value.deliveryExpiredAt, 30000, 250);
  assert.equal(ttlPending.status, "PENDING");
  const lateExpired = commandResultPayload(ttlID, "EXPIRED", { result: null, completedAt: null });
  log(`NEGATIVE/ORDER PROBE late explicit Edge EXPIRED result commandId=${ttlID}`);
  await publishMQTT(commandFilter.replace("/command", "/command-result"), lateExpired);
  const expiredDetail = await waitCommand(ttlID, (value) => value?.status === "EXPIRED", "explicit Edge EXPIRED after Cloud delivery expiry");
  assert.ok(expiredDetail.deliveryExpiredAt);
  log(JSON.stringify({ scenario: "08-no-result-ttl-then-explicit-expired", status: "PASS", commandId: ttlID, deliveryExpiredAt: ttlPending.deliveryExpiredAt }));

  await restartCollector();
  const acceptedRestartID = randomUUID();
  const acceptedStart = resultSubscriber.subscriber.messages.length;
  const acceptedBaseline = simulatorWriteCount();
  await postCommand(acceptedRestartID, device.deviceId, { hour: 15, minute: 25, second: 35, holdForRestart: true });
  await waitResult(acceptedRestartID, "ACCEPTED", acceptedStart);
  await waitCommand(acceptedRestartID, (value) => value?.status === "ACCEPTED", "Cloud ACCEPTED before restart");
  await stopBroker();
  assert.equal(simulatorWriteCount(), acceptedBaseline, "control completed before broker outage");
  await stopProcess(cloudProcess, "SIGKILL");
  cloudProcess = undefined;
  await waitForHTTPDown(`${cloudURL}/health`);
  await eventually("Collector completes accepted command while Cloud is down", () => simulatorWriteCount(), (count) => count === acceptedBaseline + 1, 60000);
  await eventually("Collector FINAL persisted while Broker is down", async () => Number((await postgresQuery(collectorDatabase,
    `SELECT count(*) FROM mqtt_outbox WHERE command_id=${sqlLiteral(acceptedRestartID)} AND message_type='COMMAND_RESULT'`)).trim()), (count) => count === 1, 30000);
  await stopProcess(collectorProcess, "SIGTERM");
  collectorProcess = undefined;
  await waitForHTTPDown(`${collectorURL}/health`);
  await startBroker();
  await startCloud();
  await waitForHTTP(`${cloudURL}/ready`, cloudProcess);
  cloudToken = await login(cloudURL, "admin", "admin123");
  await startCollector();
  collectorToken = await login(collectorURL, "admin", "admin123");
  await configureCollector();
  await waitCommand(acceptedRestartID, (value) => value?.status === "SUCCEEDED", "accepted command after Cloud restart");
  assert.equal(simulatorWriteCount(), acceptedBaseline + 1);
  log(JSON.stringify({ scenario: "09-accepted-cloud-restart", status: "PASS", commandId: acceptedRestartID, modbusWrites: simulatorWriteCount() - acceptedBaseline }));

  await runUIAcceptance(device.deviceId);
  log(JSON.stringify({ scenario: "10-browser-ui-list-detail-polling", status: "PASS", cloudURL, route: "/command" }));

  const summary = {
    status: "passed",
    scenarios: 10,
    database: "real-postgresql",
    broker: "eclipse-mosquitto:2",
    collector: "real-edge-collector",
    simulator: "real-modbus-simulator",
    cloudDeviceID: device.deviceId,
    simulatorControlWrites: simulatorWriteCount(),
    simulatorPolls: simulatorPollCount(),
  };
  console.log(JSON.stringify(summary));
}

function diagnostics() {
  const records = [];
  for (const [name, child] of [["cloud", cloudProcess], ["collector", collectorProcess], ["simulator", simulatorProcess], ["web", webProcess], ["mqtt-result-subscriber", resultSubscriber?.process]]) {
    if (child) records.push(`${name}:\n${redact(child.output)}`);
  }
  return records.join("\n");
}

async function printComposeDiagnostics() {
  if (!composeStarted) return;
  try {
    const output = await compose("logs", "--no-color", "--tail", "200", "broker", "postgres");
    process.stderr.write(`[m6-command] compose diagnostics:\n${redact(output)}\n`);
  } catch (error) {
    process.stderr.write(`[m6-command] compose diagnostics unavailable: ${redact(error.message)}\n`);
  }
}

async function cleanup() {
  if (!cleanupEnabled) {
    log(`cleanup disabled; compose project ${composeProject} and child processes were left running by request`);
    return;
  }
  if (resultSubscriber?.process) await stopProcess(resultSubscriber.process, "SIGTERM");
  await stopProcess(webProcess, "SIGTERM");
  await stopProcess(simulatorProcess, "SIGTERM");
  await stopProcess(collectorProcess, "SIGTERM");
  await stopProcess(cloudProcess, "SIGTERM");
  for (const child of [...children]) await stopProcess(child, "SIGTERM");
  if (composeStarted) {
    try { await compose("down", "--remove-orphans", "--volumes"); } catch (error) { log(`Compose cleanup failed: ${redact(error.message)}`); }
  }
  if (tempRoot) await rm(tempRoot, { recursive: true, force: true });
}

async function dependencyCheck() {
  const missing = [];
  if (!hasCommand("docker", ["info"])) missing.push("docker daemon");
  if (!hasCommand("go", ["version"])) missing.push("go");
  if (!hasCommand("uv", ["--version"])) missing.push("uv");
  for (const [label, directory] of [["server", serverDir], ["collector API", collectorDir], ["collector simulator", simulatorDir], ["collector compose fixture", composeFile]]) {
    try { await access(directory); } catch { missing.push(label); }
  }
  try {
    const browser = loadPlaywrightChromium();
    const executablePath = process.env.M6_PLAYWRIGHT_EXECUTABLE_PATH ?? browser.executablePath();
    await access(executablePath);
  } catch {
    missing.push("Playwright/Chromium");
  }
  return missing;
}

const missing = await dependencyCheck();
if (missing.length > 0) {
  const message = `SKIP M6 Command acceptance: missing ${missing.join(", ")}`;
  if (required) {
    process.stderr.write(`${message}\n`);
    process.exitCode = 1;
  } else {
    console.log(message);
  }
} else {
  try {
    await runAcceptance();
  } catch (error) {
    process.stderr.write(`[m6-command] FAIL: ${redact(error.stack ?? error)}\n`);
    const output = diagnostics();
    if (output) process.stderr.write(`[m6-command] process diagnostics:\n${output}\n`);
    await printComposeDiagnostics();
    process.exitCode = 1;
  } finally {
    await cleanup();
  }
}
