/*
 * M3 real Device-domain acceptance harness.
 *
 * The lifecycle messages in this script come from two real Edge Collector
 * processes. The only direct MQTT publications are negative raw/event probes
 * for unknown identities; this script never fabricates a normal DeviceStatus.
 * A real Modbus simulator supplies the acquisition endpoint. The simulator is
 * restarted with one deliberately slow Unit 1 to produce a real Collector
 * DEGRADED -> OFFLINE transition while Unit 2 remains healthy.
 *
 * Run from edge-platform:
 *   node scripts/m3-device-acceptance.mjs
 */
import assert from "node:assert/strict";
import { access, mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { spawn, spawnSync } from "node:child_process";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";

const repoRoot = fileURLToPath(new URL("../", import.meta.url));
const infraDir = path.resolve(
  process.env.M3_DEVICE_INFRA_ROOT
    ?? process.env.M3_DEVICE_INFRA_DIR
    ?? path.resolve(repoRoot, "..", "edge-dev-infra"),
);
const collectorRoot = path.resolve(
  process.env.M3_DEVICE_COLLECTOR_ROOT
    ?? process.env.M3_DEVICE_COLLECTOR_DIR
    ?? path.resolve(repoRoot, "..", "edge-collector"),
);
const collectorDir = path.basename(collectorRoot) === "edge-collector-api"
  ? collectorRoot
  : path.join(collectorRoot, "edge-collector-api");
const simulatorDir = path.join(collectorRoot, "modbus-simulator");
const serverDir = path.join(repoRoot, "server");

const postgresPort = numberEnv("M3_DEVICE_POSTGRES_PORT", 15432);
const mqttPort = numberEnv("M3_DEVICE_MQTT_PORT", 18884);
const cloudPort = numberEnv("M3_DEVICE_CLOUD_PORT", 18199);
const collectorAPort = numberEnv("M3_DEVICE_COLLECTOR_A_PORT", 18198);
const collectorBPort = numberEnv("M3_DEVICE_COLLECTOR_B_PORT", 18197);
const cleanupEnabled = process.env.M3_DEVICE_CLEANUP !== "0";
const edgeA = process.env.M3_DEVICE_EDGE_A ?? "m3-acceptance-edge-a";
const edgeB = process.env.M3_DEVICE_EDGE_B ?? "m3-acceptance-edge-b";
const sourceDeviceID = process.env.M3_DEVICE_SOURCE_ID ?? "m3-source-device";
const healthyDeviceID = process.env.M3_DEVICE_HEALTHY_SOURCE_ID ?? "m3-healthy-device";
const topicPrefix = process.env.M3_DEVICE_TOPIC_PREFIX ?? "edge";
const masterSecret = process.env.M3_DEVICE_MASTER_SECRET
  ?? "m3-device-acceptance-master-secret";
const composeProject = (process.env.M3_DEVICE_COMPOSE_PROJECT ?? `m3-device-acceptance-${process.pid}`)
  .toLowerCase()
  .replace(/[^a-z0-9_-]/g, "-");

const cloudDB = {
  user: process.env.M3_DEVICE_CLOUD_DB_USER ?? "edge_platform",
  password: process.env.M3_DEVICE_CLOUD_DB_PASSWORD ?? "edge-platform-db-dev",
  database: process.env.M3_DEVICE_CLOUD_DB_NAME ?? "edge_platform",
};
const collectorDBs = [
  {
    user: process.env.M3_DEVICE_COLLECTOR_A_DB_USER ?? "edge_collector",
    password: process.env.M3_DEVICE_COLLECTOR_A_DB_PASSWORD ?? "edge-collector-db-dev",
    database: process.env.M3_DEVICE_COLLECTOR_A_DB_NAME ?? "edge_collector",
  },
  {
    user: process.env.M3_DEVICE_COLLECTOR_B_DB_USER ?? "edge_collector_b",
    password: process.env.M3_DEVICE_COLLECTOR_B_DB_PASSWORD ?? "edge-collector-b-db-dev",
    database: process.env.M3_DEVICE_COLLECTOR_B_DB_NAME ?? "edge_collector_b",
  },
];
const mqttCredentials = {
  cloudUser: process.env.M3_DEVICE_CLOUD_MQTT_USER ?? "edge_platform",
  cloudPassword: process.env.M3_DEVICE_CLOUD_MQTT_PASSWORD ?? "edge-platform-mqtt-dev",
  collectorUser: process.env.M3_DEVICE_COLLECTOR_MQTT_USER ?? "edge_collector",
  collectorPassword: process.env.M3_DEVICE_COLLECTOR_MQTT_PASSWORD ?? "edge-collector-mqtt-dev",
};
const infraAdmin = {
  user: process.env.M3_DEVICE_POSTGRES_ADMIN_USER,
  password: process.env.M3_DEVICE_POSTGRES_ADMIN_PASSWORD,
};

function defaultRegisterBlocks() {
  return [{ name: "holding-sample", functionCode: 3, startAddress: 0, quantity: 2, sortOrder: 0 }];
}

const children = new Set();
const subscribers = new Set();
let tempRoot;
let composeStarted = false;
let cloudProcess;
let collectorAProcess;
let collectorBProcess;
let simulatorProcess;
let cloudBinary;
let collectorBinary;
let simulatorConfigPath;
let cloudToken;
let collectorAToken;
let collectorBToken;

function numberEnv(name, fallback) {
  const value = Number(process.env[name] ?? fallback);
  if (!Number.isInteger(value) || value < 1 || value > 65535) {
    throw new Error(`${name} must be a TCP port, got ${process.env[name]}`);
  }
  return value;
}

function log(message) {
  process.stdout.write(`[m3-device] ${message}\n`);
}

function hasCommand(command, args = ["--version"]) {
  return spawnSync(command, args, { stdio: "ignore" }).status === 0;
}

function startProcess(command, args, cwd, environment = {}) {
  const child = spawn(command, args, {
    cwd,
    env: { ...process.env, ...environment },
    detached: true,
    stdio: ["ignore", "pipe", "pipe"],
  });
  child.output = "";
  const collect = (chunk) => {
    child.output = `${child.output}${chunk}`.slice(-100000);
  };
  child.stdout?.on("data", collect);
  child.stderr?.on("data", collect);
  children.add(child);
  return child;
}

function signalProcessGroup(child, signal) {
  if (!child?.pid) return;
  try { process.kill(-child.pid, signal); } catch { /* already exited */ }
  try { process.kill(child.pid, signal); } catch { /* already exited */ }
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
    throw new Error(`${command} ${args.join(" ")} failed (${result.code ?? result.signal}):\n${child.output}`);
  }
  return child.output;
}

function composeArgs(...args) {
  const files = ["--file", "compose.yaml"];
  if ((process.env.M3_DEVICE_COMPOSE_MODE ?? "integration") === "integration") {
    files.push("--file", "compose.integration.yaml");
  }
  return ["compose", "--project-name", composeProject, ...files, ...args];
}

const composeEnvironment = {
  INTEGRATION_POSTGRES_PORT: String(postgresPort),
  INTEGRATION_MQTT_PORT: String(mqttPort),
};

async function compose(...args) {
  return runCommand("docker", composeArgs(...args), infraDir, composeEnvironment);
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
    await delay(250);
  }
  throw new Error(`TCP endpoint did not become ready: ${host}:${port}`);
}

async function waitForPath(file, timeout = 30000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try {
      await access(file);
      return;
    } catch {
      await delay(100);
    }
  }
  throw new Error(`path did not become ready: ${file}`);
}

async function waitForHTTP(url, child, timeout = 60000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(2000) });
      if (response.ok) return;
    } catch {
      // The process is still starting.
    }
    if (child && child.exitCode !== null) {
      throw new Error(`process exited while waiting for ${url}:\n${child.output}`);
    }
    await delay(250);
  }
  throw new Error(`endpoint did not become ready: ${url}\n${child?.output ?? ""}`);
}

async function waitForHTTPDown(url, timeout = 10000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try {
      await fetch(url, { signal: AbortSignal.timeout(500) });
    } catch {
      return;
    }
    await delay(100);
  }
  throw new Error(`endpoint remained reachable after termination: ${url}`);
}

async function eventually(label, read, predicate, timeout = 60000, interval = 250) {
  const deadline = Date.now() + timeout;
  let value;
  while (Date.now() < deadline) {
    value = await read();
    if (predicate(value)) return value;
    await delay(interval);
  }
  throw new Error(`${label} did not reach the expected state: ${JSON.stringify(value)}`);
}

async function waitForComposeHealthy(service, timeout = 90000) {
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

async function loadInfraAdminCredentials() {
  if (infraAdmin.user && infraAdmin.password) return;
  const envPath = path.join(infraDir, ".env");
  const contents = await readFile(envPath, "utf8").catch((error) => {
    throw new Error(`cannot read ${envPath}; set M3_DEVICE_POSTGRES_ADMIN_USER/PASSWORD: ${error.message}`);
  });
  const values = new Map();
  for (const line of contents.split("\n")) {
    const match = line.match(/^([A-Z_][A-Z0-9_]*)=(.*)$/);
    if (match) values.set(match[1], match[2].trim().replace(/^['"]|['"]$/g, ""));
  }
  infraAdmin.user ??= values.get("POSTGRES_ADMIN_USER");
  infraAdmin.password ??= values.get("POSTGRES_ADMIN_PASSWORD");
  if (!infraAdmin.user || !infraAdmin.password) {
    throw new Error(`infra admin credentials are missing; configure ${envPath} or set M3_DEVICE_POSTGRES_ADMIN_USER/PASSWORD`);
  }
}

function sqlLiteral(value) {
  return value.replaceAll("'", "''");
}

function sqlIdentifier(value) {
  return `"${value.replaceAll('"', '""')}"`;
}

function postgresExecArgs(...args) {
  return composeArgs("exec", "-T", "postgres", ...args);
}

async function alignPostgresCredentials() {
  await loadInfraAdminCredentials();
  const adminPSQL = (...args) => runCommand(
    "docker",
    postgresExecArgs("env", `PGPASSWORD=${infraAdmin.password}`, "psql", "-h", "127.0.0.1", "-U", infraAdmin.user, ...args),
    infraDir,
    composeEnvironment,
  );
  const databases = [cloudDB, ...collectorDBs];
  const roleSQL = databases.map((database) => {
    const role = sqlIdentifier(database.user);
    const roleLiteral = sqlLiteral(database.user);
    const password = sqlLiteral(database.password);
    return `DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${roleLiteral}') THEN CREATE ROLE ${role} LOGIN PASSWORD '${password}'; END IF; END $$; ALTER ROLE ${role} LOGIN PASSWORD '${password}'`;
  }).join("; ");
  await adminPSQL("-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", roleSQL);
  for (const database of databases) {
    const exists = (await adminPSQL(
      "-d", "postgres", "-At", "-c",
      `SELECT 1 FROM pg_database WHERE datname='${sqlLiteral(database.database)}'`,
    )).trim();
    const databaseName = sqlIdentifier(database.database);
    const owner = sqlIdentifier(database.user);
    if (!exists) {
      await adminPSQL("-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", `CREATE DATABASE ${databaseName} OWNER ${owner}`);
    } else {
      await adminPSQL("-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", `ALTER DATABASE ${databaseName} OWNER TO ${owner}`);
    }
  }
}

function cloudEnvironment() {
  return {
    APP_ENV: "dev",
    APP_DATABASE__DRIVER: "postgres",
    APP_DATABASE__URL: databaseURL(cloudDB.database),
    APP_DATABASE__USERNAME: cloudDB.user,
    APP_DATABASE__PASSWORD: cloudDB.password,
    APP_HTTP__ADDRESS: `127.0.0.1:${cloudPort}`,
    APP_JWT__SECRET: "m3-device-acceptance-cloud-jwt",
    APP_FILE__STORAGE_ROOT: path.join(tempRoot, "cloud-files"),
    APP_LOG__LEVEL: "warn",
    APP_LOG__FORMAT: "text",
    APP_MQTT__ENABLED: "true",
    APP_MQTT__URL: `mqtt://127.0.0.1:${mqttPort}`,
    APP_MQTT__PROTOCOL: "mqtt5",
    APP_MQTT__CLIENT_ID: "m3-device-acceptance-cloud",
    APP_MQTT__PREFIX: topicPrefix,
    APP_MQTT__USERNAME: mqttCredentials.cloudUser,
    APP_MQTT__PASSWORD: mqttCredentials.cloudPassword,
    GOCACHE: path.join(tempRoot, "go-cache-cloud"),
  };
}

function collectorEnvironment(index) {
  const database = collectorDBs[index];
  const port = index === 0 ? collectorAPort : collectorBPort;
  return {
    APP_ENV: "dev",
    APP_DATABASE__DRIVER: "postgres",
    APP_DATABASE__URL: databaseURL(database.database),
    APP_DATABASE__USERNAME: database.user,
    APP_DATABASE__PASSWORD: database.password,
    APP_HTTP__ADDRESS: `127.0.0.1:${port}`,
    APP_JWT__SECRET: `m3-device-acceptance-collector-${index}-jwt`,
    APP_FILE__STORAGE_ROOT: path.join(tempRoot, `collector-${index}-files`),
    APP_LOG__LEVEL: "warn",
    APP_LOG__FORMAT: "text",
    APP_MQTT__MASTER_SECRET: masterSecret,
    GOCACHE: path.join(tempRoot, `go-cache-collector-${index}`),
  };
}

async function startInfrastructure() {
  await access(path.join(infraDir, "compose.yaml"));
  await access(path.join(infraDir, "compose.integration.yaml"));
  await compose("up", "-d", "--remove-orphans", "postgres", "mqtt");
  composeStarted = true;
  await waitForPort("127.0.0.1", postgresPort);
  await waitForPort("127.0.0.1", mqttPort);
  await waitForComposeHealthy("postgres");
  await waitForComposeHealthy("mqtt");
  await alignPostgresCredentials();
  log(`isolated infrastructure healthy (PostgreSQL ${postgresPort}, Mosquitto ${mqttPort}, project ${composeProject})`);
}

async function migrateAndBuild() {
  await runCommand("go", ["run", "./cmd/migrate", "up", "--kind", "all"], serverDir, cloudEnvironment());
  cloudBinary = path.join(tempRoot, "edge-platform-api");
  await runCommand("go", ["build", "-o", cloudBinary, "./cmd/api"], serverDir, cloudEnvironment());
  collectorBinary = path.join(tempRoot, "edge-collector-api");
  await runCommand("go", ["build", "-o", collectorBinary, "./cmd/api"], collectorDir, collectorEnvironment(0));
  for (let index = 0; index < collectorDBs.length; index += 1) {
    await runCommand(
      "go",
      ["run", "./cmd/migrate", "up", "--kind", "all"],
      collectorDir,
      collectorEnvironment(index),
    );
  }
}

async function startCloud() {
  cloudProcess = startProcess(cloudBinary, [], serverDir, cloudEnvironment());
  await waitForHTTP(`http://127.0.0.1:${cloudPort}/health`, cloudProcess);
  log(`real Cloud Server healthy on ${cloudPort}`);
}

async function startCollector(index) {
  const process = startProcess(collectorBinary, [], collectorDir, collectorEnvironment(index));
  const port = index === 0 ? collectorAPort : collectorBPort;
  await waitForHTTP(`http://127.0.0.1:${port}/health`, process);
  if (index === 0) collectorAProcess = process;
  else collectorBProcess = process;
  log(`real Edge Collector ${index === 0 ? "A" : "B"} healthy on ${port}`);
}

async function restartCloud() {
  await stopProcess(cloudProcess, "SIGTERM");
  cloudProcess = undefined;
  await waitForHTTPDown(`http://127.0.0.1:${cloudPort}/health`);
  await startCloud();
}

async function requestJSON(baseURL, pathname, token, options = {}) {
  const response = await fetch(`${baseURL}${pathname}`, {
    ...options,
    signal: options.signal ?? AbortSignal.timeout(10000),
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

async function login(baseURL) {
  const result = await requestJSON(baseURL, "/api/auth/login", "", {
    method: "POST",
    body: { username: "admin", password: "admin123" },
  });
  assert.equal(result.response.status, 200, `login failed: ${JSON.stringify(result.payload)}`);
  assert.equal(result.payload.code, 200, `login failed: ${JSON.stringify(result.payload)}`);
  assert.ok(result.payload.data?.tokenValue, `login returned no token: ${JSON.stringify(result.payload)}`);
  return result.payload.data.tokenValue;
}

async function apiJSON(baseURL, pathname, token, options = {}) {
  const result = await requestJSON(baseURL, pathname, token, options);
  assert.equal(result.response.status, 200, `${options.method ?? "GET"} ${pathname}: ${JSON.stringify(result.payload)}`);
  assert.equal(result.payload.code, 200, `${options.method ?? "GET"} ${pathname}: ${JSON.stringify(result.payload)}`);
  return result.payload.data;
}

function collectorMQTTConfig(edgeID, clientID) {
  return {
    enabled: true,
    edgeId: edgeID,
    brokerUrl: `mqtt://127.0.0.1:${mqttPort}`,
    protocolVersion: "MQTT_5",
    clientId: clientID,
    username: mqttCredentials.collectorUser,
    passwordAction: "set",
    password: mqttCredentials.collectorPassword,
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
  };
}

async function configureCollector(index, token, edgeID, clientID) {
  const port = index === 0 ? collectorAPort : collectorBPort;
  const baseURL = `http://127.0.0.1:${port}`;
  await apiJSON(baseURL, "/api/v1/mqtt/config", token, {
    method: "PUT",
    body: collectorMQTTConfig(edgeID, clientID),
  });
  await eventually(
    `Collector ${index === 0 ? "A" : "B"} MQTT runtime connected`,
    () => apiJSON(baseURL, "/api/v1/mqtt/state", token),
    (state) => state.state === "CONNECTED" && state.connected === true,
    45000,
  );
  log(`real Collector ${index === 0 ? "A" : "B"} MQTT connected and registered as ${edgeID}`);
}

async function createCollectorChannel(index, token, name, serialPort) {
  const port = index === 0 ? collectorAPort : collectorBPort;
  return apiJSON(`http://127.0.0.1:${port}`, "/api/v1/acquisition/channels", token, {
    method: "POST",
    body: {
      name,
      protocol: "MODBUS_RTU",
      serialConfig: { port: serialPort, baudRate: 9600, dataBits: 8, stopBits: 1, parity: "N" },
      timeoutMs: 300,
      interRequestDelayMs: 0,
      enabled: 1,
    },
  });
}

async function updateCollectorChannel(index, token, channelID, serialPort) {
  const port = index === 0 ? collectorAPort : collectorBPort;
  const baseURL = `http://127.0.0.1:${port}`;
  const current = await apiJSON(baseURL, `/api/v1/acquisition/channels/${channelID}`, token);
  return apiJSON(baseURL, `/api/v1/acquisition/channels/${channelID}`, token, {
    method: "PUT",
    body: {
      name: current.name,
      protocol: current.protocol,
      serialConfig: { ...current.serialConfig, port: serialPort },
      timeoutMs: current.timeoutMs,
      interRequestDelayMs: current.interRequestDelayMs,
      enabled: current.enabled,
    },
  });
}

async function createCollectorDevice(index, token, channelID, externalID, name, unitID, pollIntervalMS = 1000, registerBlocks = defaultRegisterBlocks()) {
  const port = index === 0 ? collectorAPort : collectorBPort;
  return apiJSON(`http://127.0.0.1:${port}`, "/api/v1/acquisition/devices", token, {
    method: "POST",
    body: {
      externalId: externalID,
      name,
      deviceType: "FEED_PROTECTOR",
      channelId: channelID,
      unitId: unitID,
      pollIntervalMs: pollIntervalMS,
      failureThreshold: 3,
      enabled: 1,
      registerBlocks,
    },
  });
}

async function enableCollectorDevice(index, token, deviceID, registerBlocks = defaultRegisterBlocks()) {
  const port = index === 0 ? collectorAPort : collectorBPort;
  const baseURL = `http://127.0.0.1:${port}`;
  const current = await apiJSON(baseURL, `/api/v1/acquisition/devices/${deviceID}`, token);
  return apiJSON(baseURL, `/api/v1/acquisition/devices/${deviceID}`, token, {
    method: "PUT",
    body: {
      externalId: current.externalId,
      name: current.name,
      deviceType: current.deviceType,
      channelId: current.channelId,
      unitId: current.unitId,
      pollIntervalMs: 1000,
      failureThreshold: current.failureThreshold,
      enabled: 1,
      registerBlocks,
    },
  });
}

async function collectorState(index, token, deviceID) {
  const port = index === 0 ? collectorAPort : collectorBPort;
  return apiJSON(`http://127.0.0.1:${port}`, `/api/v1/acquisition/states/${deviceID}`, token);
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
        payload: JSON.parse(rawPayload),
        receivedAt: Date.now(),
      });
    } catch {
      // Ignore non-JSON broker output.
    }
  }

  async waitFor(predicate, timeout = 60000) {
    const deadline = Date.now() + timeout;
    while (Date.now() < deadline) {
      const message = this.messages.find(predicate);
      if (message) return message;
      if (this.child.exitCode !== null) throw new Error(`MQTT subscriber exited:\n${this.child.output}`);
      await delay(100);
    }
    throw new Error(`MQTT message did not arrive; received ${this.messages.length}: ${JSON.stringify(this.messages)}`);
  }
}

function brokerExecArgs(...args) {
  return composeArgs("exec", "-T", "mqtt", ...args);
}

function mqttSubArgs(topic) {
  return brokerExecArgs(
    "mosquitto_sub", "-V", "mqttv5", "-h", "127.0.0.1", "-p", "1883",
    "-u", mqttCredentials.cloudUser, "-P", mqttCredentials.cloudPassword,
    "-t", topic, "-q", "1", "-F", "%t|%q|%r|%p",
  );
}

async function createSubscriber(topic) {
  const process = startProcess("docker", mqttSubArgs(topic), infraDir, composeEnvironment);
  const subscriber = new MQTTSubscriber(process);
  subscribers.add(process);
  await delay(500);
  if (process.exitCode !== null) throw new Error(`MQTT subscriber failed:\n${process.output}`);
  return { process, subscriber };
}

async function publishNegativeProbe(edgeID, deviceID, kind) {
  const suffix = kind === "raw" ? "raw" : "event";
  const schema = kind === "raw" ? "raw-register-snapshot/v1" : "device-event/v1";
  const data = kind === "raw"
    ? { communicationStatus: "ONLINE", blocks: [] }
    : { kind: "m3-negative-probe", key: `unknown-${Date.now()}`, scriptId: 1, scriptVersionId: 1, payload: { probe: true } };
  const payload = JSON.stringify({
    schema,
    messageId: `m3-negative-${kind}-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    edgeId: edgeID,
    deviceId: deviceID,
    timestamp: new Date().toISOString(),
    data,
  });
  const qos = kind === "raw" ? "0" : "1";
  await runCommand(
    "docker",
    brokerExecArgs(
      "mosquitto_pub", "-V", "mqttv5", "-h", "127.0.0.1", "-p", "1883",
      "-u", mqttCredentials.collectorUser, "-P", mqttCredentials.collectorPassword,
      "-t", `${topicPrefix}/${edgeID}/device/${deviceID}/${suffix}`,
      "-q", qos, "-m", payload,
    ),
    infraDir,
    composeEnvironment,
  );
}

async function writeSimulatorConfig(faultyUnitOne) {
  const configRoot = path.join(tempRoot, "modbus-simulator");
  const devicesRoot = path.join(configRoot, "devices");
  await mkdir(devicesRoot, { recursive: true });
  const sourceRoot = path.join(simulatorDir, "config", "devices");
  let deviceOne = await readFile(path.join(sourceRoot, "feeder_protector_01.yaml"), "utf8");
  const deviceTwo = await readFile(path.join(sourceRoot, "feeder_protector_02.yaml"), "utf8");
  deviceOne = deviceOne.replace(
    /response_delay_ms:\s*\d+/,
    `response_delay_ms: ${faultyUnitOne ? 1200 : 0}`,
  );
  await writeFile(path.join(devicesRoot, "device-01.yaml"), deviceOne);
  await writeFile(path.join(devicesRoot, "device-02.yaml"), deviceTwo);
  const aliasA = path.join(os.tmpdir(), `m3-device-${process.pid}-rtu0`);
  const aliasHealthy = path.join(os.tmpdir(), `m3-device-${process.pid}-rtu1`);
  const aliasB = path.join(os.tmpdir(), `m3-device-${process.pid}-rtu2`);
  const config = (channels) => `logging:
  level: INFO
  hex: false
channels:
${channels.map(({ name, alias, deviceFile }) => `  - name: ${name}
    protocol: rtu
    alias: ${alias}
    baudrate: 9600
    bytesize: 8
    parity: N
    stopbits: 1
    devices:
      - devices/${deviceFile}
`).join("")}`;
  const simulatorConfig = path.join(configRoot, faultyUnitOne ? "faulty.yaml" : "healthy.yaml");
  await writeFile(simulatorConfig, config([
    { name: "m3-rtu0", alias: aliasA, deviceFile: "device-01.yaml" },
    { name: "m3-rtu1", alias: aliasHealthy, deviceFile: "device-02.yaml" },
    { name: "m3-rtu2", alias: aliasB, deviceFile: "device-01.yaml" },
  ]));
  simulatorConfigPath = simulatorConfig;
  return { aliasA, aliasHealthy, aliasB };
}

async function startSimulator(faultyUnitOne) {
  const aliases = await writeSimulatorConfig(faultyUnitOne);
  simulatorProcess = startProcess(
    "uv",
    ["run", "modbus-simulator", "--config", simulatorConfigPath],
    simulatorDir,
    { UV_CACHE_DIR: path.join(tempRoot, "uv-cache-main") },
  );
  await waitForPath(aliases.aliasA);
  await waitForPath(aliases.aliasHealthy);
  await waitForPath(aliases.aliasB);
  log(`real Modbus simulator healthy (${faultyUnitOne ? "Unit 1 delayed" : "all units healthy"})`);
}

async function cloudDevicePage(filters = {}) {
  const params = new URLSearchParams({ page: "1", pageSize: "100" });
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== "") params.set(key, value);
  }
  return apiJSON(`http://127.0.0.1:${cloudPort}`, `/api/device/page?${params}`, cloudToken);
}

async function cloudDeviceFor(edgeID, sourceID) {
  const page = await cloudDevicePage({ edgeId: edgeID, sourceDeviceId: sourceID });
  return page.records?.[0] ?? null;
}

async function cloudDeviceDetail(deviceID) {
  return apiJSON(`http://127.0.0.1:${cloudPort}`, `/api/device/${encodeURIComponent(deviceID)}`, cloudToken);
}

async function waitCloudDevice(edgeID, sourceID, status, timeout = 60000) {
  return eventually(
    `Cloud Device ${edgeID}/${sourceID} ${status}`,
    () => cloudDeviceFor(edgeID, sourceID),
    (value) => value?.communicationStatus === status,
    timeout,
  );
}

async function waitCollectorDevice(index, token, deviceID, status, timeout = 60000) {
  return eventually(
    `Collector ${index === 0 ? "A" : "B"} device ${deviceID} ${status}`,
    () => collectorState(index, token, deviceID),
    (value) => value?.status === status,
    timeout,
  );
}

async function psql(database, user, password, query) {
  return (await runCommand(
    "docker",
    postgresExecArgs(
      "env", `PGPASSWORD=${password}`, "psql", "-h", "127.0.0.1", "-U", user,
      "-d", database, "-At", "-c", query,
    ),
    infraDir,
    composeEnvironment,
  )).trim();
}

async function assertRESTAndPostgresProjection(device) {
  const detail = await cloudDeviceDetail(device.deviceId);
  const page = await cloudDevicePage({ edgeId: device.edgeId, sourceDeviceId: device.sourceDeviceId });
  assert.equal(page.total, 1, `identity page returned ${page.total} rows: ${JSON.stringify(page)}`);
  assert.deepEqual(page.records[0], detail, "Device page/detail projections differ");
  const rows = await psql(
    cloudDB.database,
    cloudDB.user,
    cloudDB.password,
    `SELECT json_build_object(
      'deviceId', device_id, 'edgeId', edge_id, 'sourceDeviceId', source_device_id,
      'communicationStatus', communication_status, 'registeredAt', registered_at,
      'lastSeenAt', last_seen_at, 'lastAttemptAt', last_attempt_at,
      'lastSuccessAt', last_success_at, 'communicationError', communication_error
    )::text FROM device WHERE device_id='${sqlLiteral(device.deviceId)}'`,
  );
  const row = JSON.parse(rows);
  assert.equal(row.deviceId, detail.deviceId);
  assert.equal(row.edgeId, detail.edgeId);
  assert.equal(row.sourceDeviceId, detail.sourceDeviceId);
  assert.equal(row.communicationStatus, detail.communicationStatus);
  assert.equal(Date.parse(row.registeredAt), Date.parse(detail.registeredAt));
  assert.equal(Date.parse(row.lastSeenAt), Date.parse(detail.lastSeenAt));
  assert.equal(row.communicationError, detail.communicationError);
}

async function assertDeviceSchema() {
  const columns = (await psql(
    cloudDB.database,
    cloudDB.user,
    cloudDB.password,
    "SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name='device' ORDER BY ordinal_position",
  )).split("\n").filter(Boolean);
  assert.deepEqual(columns, [
    "device_id", "edge_id", "source_device_id", "communication_status",
    "registered_at", "last_seen_at", "last_attempt_at", "last_success_at", "communication_error",
  ], `Device schema columns changed: ${columns.join(",")}`);
  const indexes = await psql(
    cloudDB.database,
    cloudDB.user,
    cloudDB.password,
    "SELECT indexname || '|' || indexdef FROM pg_indexes WHERE schemaname='public' AND tablename='device' ORDER BY indexname",
  );
  assert.match(indexes, /uq_device_edge_source/);
  assert.match(indexes, /idx_device_edge_id/);
  assert.match(indexes, /idx_device_source_device_id/);
  const forbidden = await psql(
    cloudDB.database,
    cloudDB.user,
    cloudDB.password,
    "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' AND lower(table_name) ~ '(datapoint|current_value|device_event|raw_history|device_history|payload|staging|pending_message|status_history)' ORDER BY table_name",
  );
  assert.equal(forbidden, "", `forbidden M4+ or payload/history tables exist: ${forbidden}`);
  log("PostgreSQL schema passed: Device current projection, identity index and no future persistence surface");
}

async function createEventScript() {
  const baseURL = `http://127.0.0.1:${collectorAPort}`;
  const script = await apiJSON(baseURL, "/api/v1/acquisition/scripts", collectorAToken, {
    method: "POST",
    body: {
      name: `m3-device-event-${process.pid}`,
      description: "M3 real DeviceEvent probe",
      draftSource: `def after_poll(ctx):\n    ctx.emit_event("m3-acceptance", "real-event", {"source": "real-collector"})\n`,
    },
  });
  await apiJSON(baseURL, `/api/v1/acquisition/scripts/${script.id}/publish`, collectorAToken, { method: "POST" });
  return script.id;
}

async function bindEventScript(deviceID, scriptID) {
  return apiJSON(`http://127.0.0.1:${collectorAPort}`, `/api/v1/acquisition/devices/${deviceID}/script`, collectorAToken, {
    method: "PUT",
    body: { scriptId: scriptID },
  });
}

async function runAcceptance() {
  for (const [command, args] of [["docker", ["--version"]], ["go", ["version"]], ["uv", ["--version"]]]) {
    if (!hasCommand(command, args)) throw new Error(`required command is unavailable: ${command}`);
  }
  await access(serverDir);
  await access(collectorDir);
  await access(simulatorDir);
  tempRoot = await mkdtemp(path.join(os.tmpdir(), "m3-device-acceptance-"));

  await startInfrastructure();
  await migrateAndBuild();
  await startCloud();
  cloudToken = await login(`http://127.0.0.1:${cloudPort}`);
  await startCollector(0);
  collectorAToken = await login(`http://127.0.0.1:${collectorAPort}`);
  await configureCollector(0, collectorAToken, edgeA, "m3-acceptance-collector-a");
  await eventually(
    "Cloud M2 Edge registration A",
    () => apiJSON(`http://127.0.0.1:${cloudPort}`, `/api/edge/${encodeURIComponent(edgeA)}`, cloudToken),
    (value) => value.status === "ONLINE",
  );
  log("real Collector A completed M2 Edge registration");

  const aliases = await writeSimulatorConfig(false);
  const sourceStatus = await createSubscriber(`${topicPrefix}/${edgeA}/device/${sourceDeviceID}/status`);
  const sourceRaw = await createSubscriber(`${topicPrefix}/${edgeA}/device/${sourceDeviceID}/raw`);
  const sourceEvent = await createSubscriber(`${topicPrefix}/${edgeA}/device/${sourceDeviceID}/event`);
  const channelA = await createCollectorChannel(0, collectorAToken, "M3 simulator channel A", aliases.aliasA);
  const healthyChannelA = await createCollectorChannel(0, collectorAToken, "M3 simulator healthy channel A", aliases.aliasHealthy);
  const mainDevice = await createCollectorDevice(0, collectorAToken, channelA.id, sourceDeviceID, "M3 source device A", 1, 60000, []);
  const healthyDevice = await createCollectorDevice(0, collectorAToken, healthyChannelA.id, healthyDeviceID, "M3 healthy unit 2", 2, 60000, []);

  const initialState = await eventually(
    "real Collector INITIAL current state",
    () => collectorState(0, collectorAToken, mainDevice.id),
    (value) => value?.status === "INITIAL",
    2000,
    25,
  );
  assert.equal(initialState.externalId, sourceDeviceID);
  log("real Collector acquisition INITIAL current state observed before Modbus register configuration");

  await startSimulator(false);
  await enableCollectorDevice(0, collectorAToken, mainDevice.id);
  await enableCollectorDevice(0, collectorAToken, healthyDevice.id);
  const onlineCollectorState = await waitCollectorDevice(0, collectorAToken, mainDevice.id, "ONLINE");
  const online = await waitCloudDevice(edgeA, sourceDeviceID, "ONLINE");
  await waitCollectorDevice(0, collectorAToken, healthyDevice.id, "ONLINE");
  await waitCloudDevice(edgeA, healthyDeviceID, "ONLINE");
  await assertRESTAndPostgresProjection(online);
  log(`real acquisition produced ONLINE for ${sourceDeviceID}; Collector state and Cloud projection agree (${onlineCollectorState.status})`);

  const rawMessage = await sourceRaw.subscriber.waitFor(
    (message) => message.payload?.schema === "raw-register-snapshot/v1" && message.payload?.deviceId === sourceDeviceID,
    60000,
  );
  assert.equal(rawMessage.payload.edgeId, edgeA);
  log("real Collector raw-register-snapshot observed");

  const scriptID = await createEventScript();
  await bindEventScript(mainDevice.id, scriptID);
  const eventMessage = await sourceEvent.subscriber.waitFor(
    (message) => message.payload?.schema === "device-event/v1" && message.payload?.deviceId === sourceDeviceID,
    60000,
  );
  assert.equal(eventMessage.payload.edgeId, edgeA);
  log("real Collector DeviceEvent observed from the bound acquisition script");

  const beforeCloudRestart = await cloudDeviceDetail(online.deviceId);
  await restartCloud();
  cloudToken = await login(`http://127.0.0.1:${cloudPort}`);
  const afterCloudRestart = await waitCloudDevice(edgeA, sourceDeviceID, "ONLINE");
  assert.equal(afterCloudRestart.deviceId, beforeCloudRestart.deviceId, "Cloud restart changed deviceId");
  assert.equal(afterCloudRestart.registeredAt, beforeCloudRestart.registeredAt, "Cloud restart changed registeredAt");
  assert.ok(Date.parse(afterCloudRestart.lastSeenAt) >= Date.parse(beforeCloudRestart.lastSeenAt), "retained DeviceStatus did not advance lastSeenAt");
  const restartPage = await cloudDevicePage({ edgeId: edgeA, sourceDeviceId: sourceDeviceID });
  assert.equal(restartPage.total, 1, "retained replay duplicated the Device row");
  log("Cloud restart recovered retained DeviceStatus with stable identity and no duplicate");

  const beforeEdgeOffline = await cloudDeviceDetail(afterCloudRestart.deviceId);
  log("killing real Collector A; Device must retain its last Collector assertion while Edge goes OFFLINE");
  await stopProcess(collectorAProcess, "SIGKILL");
  collectorAProcess = undefined;
  await waitForHTTPDown(`http://127.0.0.1:${collectorAPort}/health`);
  const edgeOffline = await eventually(
    "Cloud Edge A OFFLINE",
    () => apiJSON(`http://127.0.0.1:${cloudPort}`, `/api/edge/${encodeURIComponent(edgeA)}`, cloudToken),
    (value) => value.status === "OFFLINE",
  );
  assert.equal(edgeOffline.edgeId, edgeA);
  const deviceDuringEdgeOffline = await cloudDeviceDetail(beforeEdgeOffline.deviceId);
  assert.equal(deviceDuringEdgeOffline.communicationStatus, beforeEdgeOffline.communicationStatus, "Edge OFFLINE synthesized Device OFFLINE");
  assert.equal(deviceDuringEdgeOffline.registeredAt, beforeEdgeOffline.registeredAt, "Edge OFFLINE changed Device registeredAt");
  log("Edge OFFLINE isolation passed: Device status remained the last real Collector status");

  await startCollector(0);
  collectorAToken = await login(`http://127.0.0.1:${collectorAPort}`);
  await configureCollector(0, collectorAToken, edgeA, "m3-acceptance-collector-a-reconnect");
  const reconnected = await waitCloudDevice(edgeA, sourceDeviceID, "ONLINE");
  assert.equal(reconnected.deviceId, beforeEdgeOffline.deviceId, "reconnect changed Cloud deviceId");
  assert.equal(reconnected.registeredAt, beforeEdgeOffline.registeredAt, "reconnect changed registeredAt");
  assert.equal((await cloudDevicePage({ edgeId: edgeA, sourceDeviceId: sourceDeviceID })).total, 1, "reconnect duplicated Cloud Device");
  log("same Edge reconnect restored ONLINE with one stable Cloud Device");

  const unavailableSerialPort = path.join(tempRoot, "m3-unavailable-serial");
  await updateCollectorChannel(0, collectorAToken, channelA.id, unavailableSerialPort);
  log("real Collector Unit 1 channel switched to an unavailable serial path; Unit 2 remains on the live simulator channel");
  const degradedCollector = await waitCollectorDevice(0, collectorAToken, mainDevice.id, "DEGRADED");
  const degradedCloud = await waitCloudDevice(edgeA, sourceDeviceID, "DEGRADED");
  assert.equal(degradedCloud.communicationStatus, degradedCollector.status, "Cloud computed DEGRADED independently of Collector");
  const healthyCloud = await waitCloudDevice(edgeA, healthyDeviceID, "ONLINE");
  assert.equal(healthyCloud.communicationStatus, "ONLINE", "partial simulator failure affected the healthy Unit 2 Device");
  log("real simulator partial failure produced Collector/Cloud DEGRADED while Unit 2 stayed ONLINE");

  const offlineCollector = await waitCollectorDevice(0, collectorAToken, mainDevice.id, "OFFLINE");
  const offlineCloud = await waitCloudDevice(edgeA, sourceDeviceID, "OFFLINE");
  assert.equal(offlineCloud.communicationStatus, offlineCollector.status, "Cloud computed OFFLINE independently of Collector threshold");
  log("real Collector failure threshold produced OFFLINE; Cloud only projected DeviceStatus");

  await updateCollectorChannel(0, collectorAToken, channelA.id, aliases.aliasA);
  log("real Collector Unit 1 channel restored to the live simulator PTY");
  const recoveredCollector = await waitCollectorDevice(0, collectorAToken, mainDevice.id, "ONLINE", 120000);
  const recoveredCloud = await waitCloudDevice(edgeA, sourceDeviceID, "ONLINE", 120000);
  assert.equal(recoveredCloud.communicationStatus, recoveredCollector.status);
  await assertRESTAndPostgresProjection(recoveredCloud);
  log("real simulator recovery restored Collector and Cloud Device ONLINE");

  for (const [probeEdge, probeDevice] of [[edgeA, "m3-unknown-device"], ["m3-unknown-edge", "m3-unknown-device"]]) {
    await publishNegativeProbe(probeEdge, probeDevice, "raw");
    await publishNegativeProbe(probeEdge, probeDevice, "event");
  }
  await delay(1500);
  const unknownEdge = await requestJSON(`http://127.0.0.1:${cloudPort}`, "/api/device/page?edgeId=m3-unknown-edge", cloudToken);
  assert.equal(unknownEdge.response.status, 200);
  assert.equal(unknownEdge.payload.data.total, 0, "unknown raw/event probes created a Device under unknown Edge");
  const unknownDevice = await cloudDevicePage({ edgeId: edgeA, sourceDeviceId: "m3-unknown-device" });
  assert.equal(unknownDevice.total, 0, "raw/event probes created an unknown Device under a known Edge");
  const afterNegative = await cloudDeviceDetail(recoveredCloud.deviceId);
  assert.equal(afterNegative.communicationStatus, recoveredCloud.communicationStatus);
  assert.equal(afterNegative.registeredAt, recoveredCloud.registeredAt);
  log("raw/event isolation passed: unknown probes created no Device and did not mutate the known projection");

  await startCollector(1);
  collectorBToken = await login(`http://127.0.0.1:${collectorBPort}`);
  await configureCollector(1, collectorBToken, edgeB, "m3-acceptance-collector-b");
  await eventually(
    "Cloud M2 Edge registration B",
    () => apiJSON(`http://127.0.0.1:${cloudPort}`, `/api/edge/${encodeURIComponent(edgeB)}`, cloudToken),
    (value) => value.status === "ONLINE",
  );
  const channelB = await createCollectorChannel(1, collectorBToken, "M3 simulator channel B", aliases.aliasB);
  const deviceB = await createCollectorDevice(1, collectorBToken, channelB.id, sourceDeviceID, "M3 source device B", 1);
  await waitCollectorDevice(1, collectorBToken, deviceB.id, "ONLINE");
  const secondIdentity = await waitCloudDevice(edgeB, sourceDeviceID, "ONLINE");
  assert.notEqual(secondIdentity.deviceId, recoveredCloud.deviceId, "different Edge identities shared Cloud deviceId");
  assert.equal((await cloudDevicePage({ edgeId: edgeA, sourceDeviceId: sourceDeviceID })).total, 1);
  assert.equal((await cloudDevicePage({ edgeId: edgeB, sourceDeviceId: sourceDeviceID })).total, 1);
  await assertRESTAndPostgresProjection(secondIdentity);
  log(`second real Collector discovered independent Cloud Device ${secondIdentity.deviceId} for the same source identity`);

  await assertDeviceSchema();
  log("PASS: M3 real Cloud + PostgreSQL + Mosquitto + two Edge Collectors + Modbus simulator acceptance completed");
}

function activeDiagnostics() {
  const records = [];
  for (const [name, child] of [["cloud", cloudProcess], ["collectorA", collectorAProcess], ["collectorB", collectorBProcess], ["simulator", simulatorProcess]]) {
    if (child) records.push(`${name}:\n${child.output}`);
  }
  return records.join("\n");
}

async function cleanup() {
  if (!cleanupEnabled) {
    log(`cleanup disabled; compose project ${composeProject} and child processes were left running by request`);
    return;
  }
  for (const subscriber of [...subscribers]) await stopProcess(subscriber, "SIGTERM");
  await stopProcess(simulatorProcess, "SIGTERM");
  await stopProcess(collectorBProcess, "SIGTERM");
  await stopProcess(collectorAProcess, "SIGTERM");
  await stopProcess(cloudProcess, "SIGTERM");
  for (const child of [...children]) await stopProcess(child, "SIGTERM");
  if (composeStarted) {
    try { await compose("down", "--remove-orphans", "--volumes"); } catch (error) { log(`cleanup compose failed: ${error.message}`); }
  }
  if (tempRoot) await rm(tempRoot, { recursive: true, force: true });
}

try {
  await runAcceptance();
} catch (error) {
  process.stderr.write(`[m3-device] FAIL: ${error.stack ?? error}\n`);
  const diagnostics = activeDiagnostics();
  if (diagnostics) process.stderr.write(`[m3-device] process diagnostics:\n${diagnostics}\n`);
  process.exitCode = 1;
} finally {
  await cleanup();
}
