/*
 * M2 real end-to-end acceptance harness.
 *
 * This script intentionally starts the real edge-dev-infra PostgreSQL and
 * Mosquitto services, Cloud Server, and the real edge-collector API. The
 * Collector API is configured through its authenticated MQTT management API;
 * its own MQTT client therefore creates the retained online message and the
 * broker LWT creates the offline message. The negative probes below are the
 * only messages published directly by this script.
 *
 * Run from edge-platform:
 *   node scripts/m2-edge-acceptance.mjs
 *
 * Useful overrides:
 *   M2_EDGE_INFRA_DIR, M2_EDGE_COLLECTOR_DIR
 *   M2_EDGE_POSTGRES_PORT, M2_EDGE_MQTT_PORT
 *   M2_EDGE_CLOUD_PORT, M2_EDGE_COLLECTOR_PORT
 *   M2_EDGE_CLEANUP=0 (leave the uniquely named environment running)
 */
import assert from "node:assert/strict";
import { access, mkdtemp, readFile, rm } from "node:fs/promises";
import { spawn, spawnSync } from "node:child_process";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";

const repoRoot = fileURLToPath(new URL("../", import.meta.url));
const infraDir = path.resolve(
  process.env.M2_EDGE_INFRA_ROOT
    ?? process.env.M2_EDGE_INFRA_DIR
    ?? path.resolve(repoRoot, "..", "edge-dev-infra"),
);
const collectorRoot = path.resolve(
  process.env.M2_EDGE_COLLECTOR_ROOT
    ?? process.env.M2_EDGE_COLLECTOR_DIR
    ?? path.resolve(repoRoot, "..", "edge-collector"),
);
const collectorDir = path.basename(collectorRoot) === "edge-collector-api"
  ? collectorRoot
  : path.join(collectorRoot, "edge-collector-api");
const serverDir = path.join(repoRoot, "server");

const postgresPort = numberEnv("M2_EDGE_POSTGRES_PORT", 15432);
const mqttPort = numberEnv("M2_EDGE_MQTT_PORT", 18884);
const cloudPort = numberEnv("M2_EDGE_CLOUD_PORT", 18099);
const collectorPort = numberEnv("M2_EDGE_COLLECTOR_PORT", 18098);
const cleanupEnabled = process.env.M2_EDGE_CLEANUP !== "0";
const edgeID = process.env.M2_EDGE_EDGE_ID ?? process.env.M2_EDGE_ID ?? "m2-acceptance-edge";
const clientID = process.env.M2_EDGE_CLIENT_ID ?? "m2-acceptance-collector";
const topicPrefix = process.env.M2_EDGE_TOPIC_PREFIX ?? "edge";
const masterSecret = process.env.M2_EDGE_MASTER_SECRET
  ?? process.env.M2_EDGE_COLLECTOR_MQTT_MASTER_SECRET
  ?? "m2-edge-acceptance-master-secret";
const composeProject = (process.env.M2_EDGE_COMPOSE_PROJECT ?? `m2-edge-acceptance-${process.pid}`)
  .toLowerCase()
  .replace(/[^a-z0-9_-]/g, "-");

const cloudDB = {
  user: process.env.M2_EDGE_CLOUD_DB_USER ?? "edge_platform",
  password: process.env.M2_EDGE_CLOUD_DB_PASSWORD ?? "edge-platform-db-dev",
  database: process.env.M2_EDGE_CLOUD_DB_NAME ?? "edge_platform",
};
const collectorDB = {
  user: process.env.M2_EDGE_COLLECTOR_DB_USER ?? "edge_collector",
  password: process.env.M2_EDGE_COLLECTOR_DB_PASSWORD ?? "edge-collector-db-dev",
  database: process.env.M2_EDGE_COLLECTOR_DB_NAME ?? "edge_collector",
};
const infraAdmin = {
  user: process.env.M2_EDGE_POSTGRES_ADMIN_USER,
  password: process.env.M2_EDGE_POSTGRES_ADMIN_PASSWORD,
};
const mqttCredentials = {
  cloudUser: process.env.M2_EDGE_CLOUD_MQTT_USER ?? "edge_platform",
  cloudPassword: process.env.M2_EDGE_CLOUD_MQTT_PASSWORD ?? "edge-platform-mqtt-dev",
  collectorUser: process.env.M2_EDGE_COLLECTOR_MQTT_USER ?? "edge_collector",
  collectorPassword: process.env.M2_EDGE_COLLECTOR_MQTT_PASSWORD ?? "edge-collector-mqtt-dev",
};

const children = new Set();
let tempRoot;
let composeStarted = false;
let cloudProcess;
let collectorProcess;
let subscriberProcess;
let cloudBinary;
let collectorBinary;

function numberEnv(name, fallback) {
  const value = Number(process.env[name] ?? fallback);
  if (!Number.isInteger(value) || value < 1 || value > 65535) {
    throw new Error(`${name} must be a TCP port, got ${process.env[name]}`);
  }
  return value;
}

function log(message) {
  process.stdout.write(`[m2-edge] ${message}\n`);
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
  try {
    process.kill(-child.pid, signal);
  } catch {
    // The group may have exited while cleanup was being scheduled.
  }
  try {
    process.kill(child.pid, signal);
  } catch {
    // The direct process may already have been terminated with its group.
  }
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
  if ((process.env.M2_EDGE_COMPOSE_MODE ?? "integration") === "integration") {
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

async function waitForHTTPDown(url, timeout = 5000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try {
      await fetch(url, { signal: AbortSignal.timeout(500) });
    } catch {
      return;
    }
    await delay(100);
  }
  throw new Error(`endpoint remained reachable after process termination: ${url}`);
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
  // Both applications deliberately receive credentials through their typed
  // config fields. Their database layer rejects credentials embedded in URL.
  return `postgres://127.0.0.1:${postgresPort}/${database}?sslmode=disable`;
}

async function loadInfraAdminCredentials() {
  if (infraAdmin.user && infraAdmin.password) return;
  const envPath = path.join(infraDir, ".env");
  let contents;
  try {
    contents = await readFile(envPath, "utf8");
  } catch (error) {
    throw new Error(`cannot read ${envPath}; set M2_EDGE_POSTGRES_ADMIN_USER and M2_EDGE_POSTGRES_ADMIN_PASSWORD: ${error.message}`);
  }
  const values = new Map();
  for (const line of contents.split("\n")) {
    const match = line.match(/^([A-Z_][A-Z0-9_]*)=(.*)$/);
    if (match) values.set(match[1], match[2].trim().replace(/^['"]|['"]$/g, ""));
  }
  infraAdmin.user ??= values.get("POSTGRES_ADMIN_USER");
  infraAdmin.password ??= values.get("POSTGRES_ADMIN_PASSWORD");
  if (!infraAdmin.user || !infraAdmin.password) {
    throw new Error(`infra admin credentials are missing; set M2_EDGE_POSTGRES_ADMIN_USER and M2_EDGE_POSTGRES_ADMIN_PASSWORD or configure ${envPath}`);
  }
}

async function alignPostgresCredentials() {
  await loadInfraAdminCredentials();
  const adminPSQL = async (...args) => runCommand("docker", postgresExecArgs("env", `PGPASSWORD=${infraAdmin.password}`, "psql", "-h", "127.0.0.1", "-U", infraAdmin.user, ...args), infraDir, composeEnvironment);
  const roleSQL = [cloudDB, collectorDB].map((database) => {
    const role = database.user.replaceAll('"', '""');
    const password = database.password.replaceAll("'", "''");
    return `DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${database.user.replaceAll("'", "''")}') THEN CREATE ROLE \"${role}\" LOGIN PASSWORD '${password}'; END IF; END $$; ALTER ROLE \"${role}\" LOGIN PASSWORD '${password}'`;
  }).join("; ");
  await adminPSQL("-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", roleSQL);
  for (const database of [cloudDB, collectorDB]) {
    const exists = (await adminPSQL("-d", "postgres", "-At", "-c", `SELECT 1 FROM pg_database WHERE datname='${database.database.replaceAll("'", "''")}'`)).trim();
    const dbName = database.database.replaceAll('"', '""');
    const owner = database.user.replaceAll('"', '""');
    if (!exists) await adminPSQL("-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", `CREATE DATABASE \"${dbName}\" OWNER \"${owner}\"`);
    else await adminPSQL("-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", `ALTER DATABASE \"${dbName}\" OWNER TO \"${owner}\"`);
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
    APP_JWT__SECRET: "m2-edge-acceptance-cloud-jwt",
    APP_FILE__STORAGE_ROOT: path.join(tempRoot, "cloud-files"),
    APP_LOG__LEVEL: "warn",
    APP_LOG__FORMAT: "text",
    APP_MQTT__ENABLED: "true",
    APP_MQTT__URL: `mqtt://127.0.0.1:${mqttPort}`,
    APP_MQTT__PROTOCOL: "mqtt5",
    APP_MQTT__CLIENT_ID: "m2-acceptance-cloud",
    APP_MQTT__PREFIX: topicPrefix,
    APP_MQTT__USERNAME: mqttCredentials.cloudUser,
    APP_MQTT__PASSWORD: mqttCredentials.cloudPassword,
    GOCACHE: path.join(tempRoot, "go-cache-cloud"),
  };
}

function collectorEnvironment() {
  return {
    APP_ENV: "dev",
    APP_DATABASE__DRIVER: "postgres",
    APP_DATABASE__URL: databaseURL(collectorDB.database),
    APP_DATABASE__USERNAME: collectorDB.user,
    APP_DATABASE__PASSWORD: collectorDB.password,
    APP_HTTP__ADDRESS: `127.0.0.1:${collectorPort}`,
    APP_JWT__SECRET: "m2-edge-acceptance-collector-jwt",
    APP_FILE__STORAGE_ROOT: path.join(tempRoot, "collector-files"),
    APP_LOG__LEVEL: "warn",
    APP_LOG__FORMAT: "text",
    APP_MQTT__MASTER_SECRET: masterSecret,
    GOCACHE: path.join(tempRoot, "go-cache-collector"),
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
  log(`infra healthy (PostgreSQL ${postgresPort}, Mosquitto ${mqttPort}, project ${composeProject})`);
}

async function migrateAndStartCloud() {
  const env = cloudEnvironment();
  await runCommand("go", ["run", "./cmd/migrate", "up", "--kind", "all"], serverDir, env);
  cloudBinary = path.join(tempRoot, "edge-platform-api");
  await runCommand("go", ["build", "-o", cloudBinary, "./cmd/api"], serverDir, env);
  cloudProcess = startProcess(cloudBinary, [], serverDir, env);
  await waitForHTTP(`http://127.0.0.1:${cloudPort}/health`, cloudProcess);
  log(`Cloud Server healthy on ${cloudPort}`);
}

async function migrateAndStartCollector() {
  const env = collectorEnvironment();
  await runCommand("go", ["run", "./cmd/migrate", "up", "--kind", "all"], collectorDir, env);
  collectorBinary = path.join(tempRoot, "edge-collector-api");
  await runCommand("go", ["build", "-o", collectorBinary, "./cmd/api"], collectorDir, env);
  collectorProcess = startProcess(collectorBinary, [], collectorDir, env);
  await waitForHTTP(`http://127.0.0.1:${collectorPort}/health`, collectorProcess);
  log(`real Edge Collector API healthy on ${collectorPort}`);
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

function collectorMQTTConfig() {
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
    rawPublishIntervalMs: 1000,
    outboxMaxRows: 100,
    outboxMaxBytes: 8 * 1024 * 1024,
    outboxRetentionDays: 7,
    commandJournalRetentionDays: 7,
    commandJournalMaxRows: 100,
    commandQueueCapacity: 8,
    commandPollFairness: 1,
  };
}

async function configureCollector(token) {
  await apiJSON(`http://127.0.0.1:${collectorPort}`, "/api/v1/mqtt/config", token, {
    method: "PUT",
    body: collectorMQTTConfig(),
  });
  await eventually(
    "Collector MQTT runtime connected",
    () => apiJSON(`http://127.0.0.1:${collectorPort}`, "/api/v1/mqtt/state", token),
    (state) => state.state === "CONNECTED" && state.connected === true,
    45000,
  );
  log("Collector MQTT configured through API; retained online is produced by the real Collector");
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
      // Ignore non-JSON broker output; valid M2 envelopes are JSON.
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
    throw new Error(`MQTT message did not arrive; received ${this.messages.length} messages: ${JSON.stringify(this.messages)}`);
  }
}

function brokerExecArgs(...args) {
  return composeArgs("exec", "-T", "mqtt", ...args);
}

function postgresExecArgs(...args) {
  return composeArgs("exec", "-T", "postgres", ...args);
}

function mqttSubArgs(topic) {
  return brokerExecArgs("mosquitto_sub", "-V", "mqttv5", "-h", "127.0.0.1", "-p", "1883",
    "-u", mqttCredentials.cloudUser, "-P", mqttCredentials.cloudPassword, "-t", topic, "-q", "1", "-F", "%t|%q|%r|%p");
}

async function startSubscriber(topic) {
  const result = await createSubscriber(topic);
  subscriberProcess = result.process;
  return result.subscriber;
}

async function createSubscriber(topic) {
  const process = startProcess("docker", mqttSubArgs(topic), infraDir, composeEnvironment);
  const subscriber = new MQTTSubscriber(process);
  await delay(500);
  if (process.exitCode !== null) throw new Error(`MQTT subscriber failed:\n${process.output}`);
  return { process, subscriber };
}

async function publishProbe(edgeIDForProbe, kind, payload) {
  const deviceID = "m2-probe-device";
  const suffix = kind === "status" ? "status" : kind === "raw" ? "raw" : "event";
  const topic = `${topicPrefix}/${edgeIDForProbe}/device/${deviceID}/${suffix}`;
  const qos = kind === "raw" ? "0" : "1";
  await runCommand("docker", brokerExecArgs("mosquitto_pub", "-V", "mqttv5", "-h", "127.0.0.1", "-p", "1883",
    "-u", mqttCredentials.collectorUser, "-P", mqttCredentials.collectorPassword, "-t", topic, "-q", qos, "-m", JSON.stringify(payload)), infraDir, composeEnvironment);
}

function validProbePayload(schema, edgeIDForProbe) {
  return {
    schema,
    messageId: `m2-negative-${schema}-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    edgeId: edgeIDForProbe,
    deviceId: "m2-probe-device",
    timestamp: new Date().toISOString(),
    data: schema === "device-status/v1"
      ? { status: "ONLINE", lastSuccessAt: null, lastAttemptAt: null, error: null }
      : schema === "raw-register-snapshot/v1"
        ? { communicationStatus: "ONLINE", blocks: [] }
        : { kind: "m2-negative-probe", key: "m2", payload: { probe: true } },
  };
}

async function runNegativeProbes(cloudToken, baseline) {
  const schemas = ["device-status/v1", "raw-register-snapshot/v1", "device-event/v1"];
  for (const edgeIDForProbe of ["m2-unknown-edge", edgeID]) {
    for (const schema of schemas) {
      const kind = schema === "device-status/v1" ? "status" : schema === "raw-register-snapshot/v1" ? "raw" : "event";
      await publishProbe(edgeIDForProbe, kind, validProbePayload(schema, edgeIDForProbe));
    }
  }
  await delay(1500);
  const unknown = await requestJSON(`http://127.0.0.1:${cloudPort}`, `/api/edge/${encodeURIComponent("m2-unknown-edge")}`, cloudToken);
  assert.equal(unknown.response.status, 404, `unknown non-Edge probes created an Edge: ${JSON.stringify(unknown.payload)}`);
  const after = await apiJSON(`http://127.0.0.1:${cloudPort}`, `/api/edge/${encodeURIComponent(edgeID)}`, cloudToken);
  assert.equal(after.status, baseline.status, "known Edge status changed after non-Edge probes");
  assert.equal(after.lastSeenAt, baseline.lastSeenAt, "known Edge lastSeenAt changed after non-Edge probes");
  log("isolation passed: unknown device-status/raw/device-event did not create Edge; known Edge projection did not change");
}

async function publishSchemaProbeIsNotCollectorLifecycle() {
  log("negative probes above used mosquitto_pub only; online/offline/reconnect are still real Collector lifecycle messages");
}

async function runSchemaAssertions() {
  const psql = async (database, user, password, query) => (await runCommand("docker", postgresExecArgs("env", `PGPASSWORD=${password}`, "psql", "-h", "127.0.0.1", "-U", user, "-d", database, "-At", "-F", "\t", "-c", query), infraDir, composeEnvironment)).trim();
  const edgeColumns = (await psql(cloudDB.database, cloudDB.user, cloudDB.password,
    "SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name='edge' ORDER BY ordinal_position"))
    .split("\n").filter(Boolean);
  assert.deepEqual(edgeColumns, ["edge_id", "status", "registered_at", "last_seen_at"], `edge schema changed: ${edgeColumns.join(",")}`);

  const forbiddenTables = await psql(cloudDB.database, cloudDB.user, cloudDB.password,
    "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' AND lower(table_name) ~ '(device|datapoint|raw|event|history)' ORDER BY table_name");
  assert.equal(forbiddenTables, "", `forbidden Edge history tables exist: ${forbiddenTables}`);

  const forbiddenColumns = await psql(cloudDB.database, cloudDB.user, cloudDB.password,
    "SELECT table_name || '.' || column_name FROM information_schema.columns WHERE table_schema='public' AND lower(column_name) ~ '(payload|source_timestamp|raw_payload|event_payload|device_id|datapoint_id)' ORDER BY 1");
  assert.equal(forbiddenColumns, "", `forbidden Edge payload/source columns exist: ${forbiddenColumns}`);
  log("PostgreSQL schema passed: edge has exactly four projection columns and no forbidden persistence surface");
}

async function countEdgeRows() {
  const output = await runCommand("docker", postgresExecArgs("env", `PGPASSWORD=${cloudDB.password}`, "psql", "-h", "127.0.0.1", "-U", cloudDB.user, "-d", cloudDB.database, "-At", "-c", "SELECT count(*) FROM edge WHERE edge_id='" + edgeID.replaceAll("'", "''") + "'"), infraDir, composeEnvironment);
  return Number(output.trim());
}

async function runAcceptance() {
  await access(serverDir);
  await access(collectorDir);
  for (const [command, args] of [["docker", ["--version"]], ["go", ["version"]]]) {
    if (!hasCommand(command, args)) throw new Error(`required command is unavailable: ${command}`);
  }
  tempRoot = await mkdtemp(path.join(os.tmpdir(), "m2-edge-acceptance-"));
  await startInfrastructure();
  await migrateAndStartCloud();
  const cloudToken = await login(`http://127.0.0.1:${cloudPort}`);
  await migrateAndStartCollector();
  const collectorToken = await login(`http://127.0.0.1:${collectorPort}`);
  const subscriber = await startSubscriber(`${topicPrefix}/${edgeID}/status`);
  await configureCollector(collectorToken);

  const onlineMessage = await subscriber.waitFor(
    (message) => message.payload?.schema === "edge-status/v1"
      && message.payload?.data?.online === true,
    60000,
  );
  log(`real Collector live ONLINE observed (retain flag ${onlineMessage.retained ? "set" : "clear"})`);

  // A retained publication delivered to the publishing connection's existing
  // subscriber has retain=0. Use a second subscriber to assert the Broker's
  // retained copy explicitly, while the first subscriber remains for LWT.
  const retainedSubscription = await createSubscriber(`${topicPrefix}/${edgeID}/status`);
  const retainedOnline = await retainedSubscription.subscriber.waitFor(
    (message) => message.payload?.schema === "edge-status/v1"
      && message.payload?.data?.online === true,
    60000,
  );
  assert.equal(retainedOnline.retained, true, `real Collector online discovery was not retained: ${JSON.stringify(retainedOnline)}`);
  await stopProcess(retainedSubscription.process, "SIGTERM");
  log("real Collector retained ONLINE message observed by Mosquitto subscriber");

  const edgeDetail = () => apiJSON(`http://127.0.0.1:${cloudPort}`, `/api/edge/${encodeURIComponent(edgeID)}`, cloudToken);
  const online = await eventually("Cloud retained ONLINE projection", edgeDetail, (value) => value.status === "ONLINE");
  assert.equal(await countEdgeRows(), 1, "retained online did not create exactly one Edge row");
  const page = await apiJSON(`http://127.0.0.1:${cloudPort}`, "/api/edge/page?page=1&pageSize=10&edgeId=" + encodeURIComponent(edgeID), cloudToken);
  assert.equal(page.total, 1, "authenticated Edge page did not find the real Collector Edge");
  assert.deepEqual(page.records[0], online, "Edge page/detail projections differ");
  log(`real Collector retained ONLINE discovered Edge ${edgeID}`);

  const killIssuedAt = Date.now();
  log("killing the real Collector API process; offline must arrive from its Mosquitto LWT");
  await stopProcess(collectorProcess, "SIGKILL");
  collectorProcess = undefined;
  await waitForHTTPDown(`http://127.0.0.1:${collectorPort}/health`);
  const offlineMessage = await subscriber.waitFor((message) => message.payload?.schema === "edge-status/v1" && message.payload?.data?.online === false, 60000);
  const lwtObservedAt = offlineMessage.receivedAt;
  const lwtTimestamp = Date.parse(offlineMessage.payload.timestamp);
  assert.ok(Number.isFinite(lwtTimestamp), `LWT timestamp is invalid: ${JSON.stringify(offlineMessage.payload)}`);
  assert.ok(lwtTimestamp < killIssuedAt, `LWT timestamp ${lwtTimestamp} was not earlier than Collector disconnect ${killIssuedAt}`);
  const offline = await eventually("Cloud LWT OFFLINE projection", edgeDetail, (value) => value.status === "OFFLINE");
  const cloudLastSeenAt = Date.parse(offline.lastSeenAt);
  assert.ok(Math.abs(cloudLastSeenAt - lwtObservedAt) <= 5000, `Cloud lastSeenAt ${offline.lastSeenAt} is not close to observed LWT ${new Date(lwtObservedAt).toISOString()}`);
  assert.equal(offline.registeredAt, online.registeredAt, "registeredAt changed on LWT");
  log(`real LWT OFFLINE received; Cloud lastSeenAt is within 5s of broker observation and LWT timestamp predates disconnect`);

  collectorProcess = startProcess(collectorBinary, [], collectorDir, collectorEnvironment());
  await waitForHTTP(`http://127.0.0.1:${collectorPort}/health`, collectorProcess);
  await eventually("Cloud real Collector reconnect ONLINE projection", edgeDetail, (value) => value.status === "ONLINE", 60000);
  const reconnected = await edgeDetail();
  assert.equal(await countEdgeRows(), 1, "reconnect created more than one Edge row");
  assert.equal(reconnected.registeredAt, online.registeredAt, "registeredAt changed after reconnect");
  log("same edgeId real Collector reconnect restored ONLINE with one row and unchanged registeredAt");

  await runNegativeProbes(cloudToken, reconnected);
  await publishSchemaProbeIsNotCollectorLifecycle();
  await runSchemaAssertions();
  log("PASS: M2 real Cloud + PostgreSQL + Mosquitto + Edge Collector acceptance completed");
}

async function cleanup() {
  if (!cleanupEnabled) {
    log(`cleanup disabled; compose project ${composeProject} and child processes were left running by request`);
    return;
  }
  await stopProcess(subscriberProcess, "SIGTERM");
  await stopProcess(collectorProcess, "SIGTERM");
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
  process.stderr.write(`[m2-edge] FAIL: ${error.stack ?? error}\n`);
  process.exitCode = 1;
} finally {
  await cleanup();
}
