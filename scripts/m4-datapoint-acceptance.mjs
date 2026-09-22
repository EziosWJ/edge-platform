/*
 * M4 acceptance probe. The normal data path is deliberately not fabricated:
 * start the real PostgreSQL/Mosquitto/Cloud/Collector/Modbus environment first
 * (the same base used by M3), then set M4_CLOUD_URL and M4_DEVICE_ID. The probe
 * creates management mappings through REST and waits for values published by
 * the real Collector.
 *
 * Besides the normal GOOD path, this probe exercises the real Collector channel
 * failure/recovery path, metadata-only updates, mapping reset, disable/enable,
 * REST filters, and (when M4_MQTT_COMPOSE_PROJECT is supplied) an unknown raw
 * negative probe plus the PostgreSQL schema boundary.
 */
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";

const cloudURL = (process.env.M4_CLOUD_URL ?? "").replace(/\/$/, "");
const deviceID = process.env.M4_DEVICE_ID;
assert.ok(cloudURL, "M4_CLOUD_URL is required");
assert.ok(deviceID, "M4_DEVICE_ID is required");

const collectorURL = (process.env.M4_COLLECTOR_URL ?? "http://127.0.0.1:18198").replace(/\/$/, "");
const runID = sanitizeRunID(process.env.M4_RUN_ID ?? String(Date.now()));
const numberAddress = integerEnv("M4_NUMBER_ADDRESS", 0);
const booleanAddress = integerEnv("M4_BOOLEAN_ADDRESS", 1);
const booleanBit = integerEnv("M4_BOOLEAN_BIT", 0);
const numberScale = finiteEnv("M4_NUMBER_SCALE", 0.1);
const numberPointKey = process.env.M4_NUMBER_POINT_KEY ?? `m4_current_a_${runID}`;
const booleanPointKey = process.env.M4_BOOLEAN_POINT_KEY ?? `m4_breaker_closed_${runID}`;
const token = process.env.M4_TOKEN ?? await login(cloudURL, process.env.M4_USERNAME, process.env.M4_PASSWORD);
const collectorToken = process.env.M4_COLLECTOR_TOKEN
  ?? await login(collectorURL, process.env.M4_COLLECTOR_USERNAME, process.env.M4_COLLECTOR_PASSWORD);

function sanitizeRunID(value) {
  const normalized = value.toLowerCase().replace(/[^a-z0-9_]/g, "_").replace(/^_+|_+$/g, "");
  return (normalized || "run").slice(0, 40);
}

function integerEnv(name, fallback) {
  const value = Number(process.env[name] ?? fallback);
  assert.ok(Number.isInteger(value) && value >= 0 && value <= 65535, `${name} must be an integer in [0, 65535]`);
  return value;
}

function finiteEnv(name, fallback) {
  const value = Number(process.env[name] ?? fallback);
  assert.ok(Number.isFinite(value), `${name} must be finite`);
  return value;
}

function delay(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

async function requestJSON(baseURL, path, authorization, options = {}) {
  const response = await fetch(`${baseURL}${path}`, {
    ...options,
    signal: options.signal ?? AbortSignal.timeout(15000),
    headers: {
      ...(options.body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...(authorization ? { Authorization: authorization.startsWith("Bearer ") ? authorization : `Bearer ${authorization}` } : {}),
      ...(options.headers ?? {}),
    },
    body: options.body !== undefined && typeof options.body !== "string" ? JSON.stringify(options.body) : options.body,
  });
  const text = await response.text();
  let payload;
  try {
    payload = JSON.parse(text);
  } catch {
    payload = { raw: text };
  }
  return { response, payload };
}

async function login(baseURL, username = "admin", password = "admin123") {
  const result = await requestJSON(baseURL, "/api/auth/login", "", {
    method: "POST",
    body: { username: username ?? "admin", password: password ?? "admin123" },
  });
  assert.equal(result.response.status, 200, `login failed at ${baseURL}: ${JSON.stringify(result.payload)}`);
  assert.equal(result.payload.code, 200, `login failed at ${baseURL}: ${JSON.stringify(result.payload)}`);
  assert.ok(result.payload.data?.tokenValue, `login returned no token at ${baseURL}: ${JSON.stringify(result.payload)}`);
  return result.payload.data.tokenValue;
}

async function cloudAPI(path, options = {}) {
  const result = await requestJSON(cloudURL, path, token, options);
  assert.equal(result.response.status, 200, `${options.method ?? "GET"} ${path}: ${JSON.stringify(result.payload)}`);
  assert.equal(result.payload.code, 200, `${options.method ?? "GET"} ${path}: ${JSON.stringify(result.payload)}`);
  return result.payload.data;
}

async function collectorAPI(path, options = {}) {
  const result = await requestJSON(collectorURL, path, collectorToken, options);
  assert.equal(result.response.status, 200, `${options.method ?? "GET"} ${path}: ${JSON.stringify(result.payload)}`);
  assert.equal(result.payload.code, 200, `${options.method ?? "GET"} ${path}: ${JSON.stringify(result.payload)}`);
  return result.payload.data;
}

async function waitForPoint(pointID, predicate, description, timeout = 90000) {
  const deadline = Date.now() + timeout;
  let last;
  while (Date.now() < deadline) {
    last = await cloudAPI(`/api/datapoint/${encodeURIComponent(pointID)}`);
    if (predicate(last)) return last;
    await delay(1000);
  }
  throw new Error(`${description} timed out for ${pointID}: ${JSON.stringify(last)}`);
}

async function waitForGood(pointID, minimumRevision = 0, timeout = 90000) {
  return waitForPoint(
    pointID,
    (point) => point.currentValue.quality === "GOOD"
      && point.currentValue.value !== null
      && point.currentValue.revision > minimumRevision,
    `real Collector did not produce GOOD CurrentValue revision > ${minimumRevision}`,
    timeout,
  );
}

async function waitForQuality(pointID, quality, minimumRevision, timeout = 90000) {
  return waitForPoint(
    pointID,
    (point) => point.currentValue.quality === quality && point.currentValue.revision > minimumRevision,
    `CurrentValue did not become ${quality} with a newer revision`,
    timeout,
  );
}

function numberMapping(address = numberAddress) {
  return {
    sourceType: "MODBUS_REGISTER",
    functionCode: 3,
    address,
    encoding: "UINT16",
    byteOrder: "BIG_ENDIAN",
    wordOrder: null,
    bitIndex: null,
    scale: numberScale,
    offset: 0,
  };
}

function booleanMapping() {
  return {
    sourceType: "MODBUS_REGISTER",
    functionCode: 3,
    address: booleanAddress,
    encoding: "BOOLEAN_BIT",
    byteOrder: "BIG_ENDIAN",
    wordOrder: null,
    bitIndex: booleanBit,
    scale: 1,
    offset: 0,
  };
}

function mappingBody(mapping, overrides = {}) {
  const { dataPointId: _dataPointID, ...clean } = mapping;
  return { ...clean, ...overrides };
}

function pointUpdateBody(point, mapping = point.mapping, name = point.name) {
  return {
    name,
    unit: point.unit,
    precision: point.precision,
    mapping: mappingBody(mapping),
  };
}

async function findCollectorSourceDevice() {
  const externalID = process.env.M4_COLLECTOR_DEVICE_EXTERNAL_ID ?? "m3-source-device";
  const page = await collectorAPI("/api/v1/acquisition/devices?page=1&pageSize=500");
  const devices = page.records ?? page.items ?? page;
  const device = devices.find((candidate) => candidate.externalId === externalID);
  assert.ok(device, `Collector source device ${externalID} was not found`);
  assert.ok(device.channelId, `Collector source device ${externalID} has no channel`);
  const channel = await collectorAPI(`/api/v1/acquisition/channels/${encodeURIComponent(device.channelId)}`);
  assert.ok(channel.serialConfig?.port, `Collector channel ${device.channelId} has no serial port`);
  return { device, channel };
}

async function updateCollectorChannel(channel, serialPort) {
  return collectorAPI(`/api/v1/acquisition/channels/${encodeURIComponent(channel.id)}`, {
    method: "PUT",
    body: {
      name: channel.name,
      protocol: channel.protocol,
      serialConfig: { ...channel.serialConfig, port: serialPort },
      timeoutMs: channel.timeoutMs,
      interRequestDelayMs: channel.interRequestDelayMs,
      enabled: channel.enabled,
    },
  });
}

async function assertRESTFilters(point) {
  const query = new URLSearchParams({
    page: "1",
    pageSize: "500",
    deviceId: deviceID,
    pointKey: point.pointKey,
    valueType: point.valueType,
    enabled: String(point.enabled),
    quality: point.currentValue.quality,
  });
  const page = await cloudAPI(`/api/datapoint/page?${query}`);
  assert.equal(page.total, 1, `exact DataPoint filters returned ${page.total} rows`);
  assert.equal(page.records[0].dataPointId, point.dataPointId);
}

async function assertMetadataUpdatePreservesCurrent(pointID) {
  const before = await cloudAPI(`/api/datapoint/${encodeURIComponent(pointID)}`);
  const after = await cloudAPI(`/api/datapoint/${encodeURIComponent(pointID)}`, {
    method: "PUT",
    body: pointUpdateBody(before, before.mapping, `${before.name} metadata`),
  });
  assert.deepEqual(after.currentValue, before.currentValue, "metadata-only update changed CurrentValue");
  return after;
}

async function assertRealFailureAndRecovery(numberPointID, booleanPointID) {
  const source = await findCollectorSourceDevice();
  const originalPort = source.channel.serialConfig.port;
  const unavailablePort = process.env.M4_UNAVAILABLE_SERIAL_PORT
    ?? `/tmp/edge-platform-m4-missing-${process.pid}-${Date.now()}`;
  assert.notEqual(unavailablePort, originalPort, "M4 unavailable serial path must differ from the live path");

  const beforeNumber = await cloudAPI(`/api/datapoint/${encodeURIComponent(numberPointID)}`);
  const beforeBoolean = await cloudAPI(`/api/datapoint/${encodeURIComponent(booleanPointID)}`);
  assert.equal(beforeNumber.currentValue.quality, "GOOD");
  assert.equal(beforeBoolean.currentValue.quality, "GOOD");

  await updateCollectorChannel(source.channel, unavailablePort);
  let badNumber;
  let badBoolean;
  try {
    badNumber = await waitForQuality(numberPointID, "BAD", beforeNumber.currentValue.revision);
    badBoolean = await waitForQuality(booleanPointID, "BAD", beforeBoolean.currentValue.revision);
  } finally {
    await updateCollectorChannel(source.channel, originalPort);
  }

  for (const [before, bad] of [[beforeNumber, badNumber], [beforeBoolean, badBoolean]]) {
    assert.equal(bad.currentValue.value, before.currentValue.value, "BAD projection did not retain last GOOD value");
    assert.equal(bad.currentValue.sourceTimestamp, before.currentValue.sourceTimestamp, "BAD projection changed sourceTimestamp");
    assert.ok(bad.currentValue.observedAt && before.currentValue.observedAt);
    assert.ok(Date.parse(bad.currentValue.observedAt) > Date.parse(before.currentValue.observedAt), "BAD observedAt did not advance");
    assert.ok(bad.currentValue.revision > before.currentValue.revision, "BAD revision did not advance");
  }

  const recoveredNumber = await waitForGood(numberPointID, badNumber.currentValue.revision);
  const recoveredBoolean = await waitForGood(booleanPointID, badBoolean.currentValue.revision);
  assert.equal(recoveredNumber.currentValue.quality, "GOOD");
  assert.equal(recoveredBoolean.currentValue.quality, "GOOD");
  return { recoveredNumber, recoveredBoolean };
}

async function assertMappingResetAndRecovery(numberPoint) {
  const remapAddress = integerEnv("M4_REMAP_ADDRESS", booleanAddress);
  assert.notEqual(remapAddress, numberAddress, "M4_REMAP_ADDRESS must differ from the initial number address");

  const disabled = await cloudAPI(`/api/datapoint/${encodeURIComponent(numberPoint.dataPointId)}/enabled`, {
    method: "PUT",
    body: { enabled: false },
  });
  assert.equal(disabled.enabled, false);
  assert.equal(disabled.currentValue.quality, "NO_DATA");
  assert.equal(disabled.currentValue.value, null);
  const disabledRevision = disabled.currentValue.revision;
  await delay(1500);
  const stillDisabled = await cloudAPI(`/api/datapoint/${encodeURIComponent(numberPoint.dataPointId)}`);
  assert.equal(stillDisabled.currentValue.revision, disabledRevision, "disabled DataPoint consumed a raw update");

  const remapped = await cloudAPI(`/api/datapoint/${encodeURIComponent(numberPoint.dataPointId)}`, {
    method: "PUT",
    body: pointUpdateBody(disabled, mappingBody(disabled.mapping, { address: remapAddress })),
  });
  assert.equal(remapped.mapping.address, remapAddress);
  assert.equal(remapped.currentValue.quality, "NO_DATA");
  assert.equal(remapped.currentValue.value, null);

  const enabled = await cloudAPI(`/api/datapoint/${encodeURIComponent(numberPoint.dataPointId)}/enabled`, {
    method: "PUT",
    body: { enabled: true },
  });
  assert.equal(enabled.enabled, true);
  const recovered = await waitForGood(numberPoint.dataPointId, enabled.currentValue.revision);
  assert.equal(recovered.mapping.address, remapAddress);
  return recovered;
}

async function assertDisableEnableRequiresNewRaw(booleanPoint) {
  const disabled = await cloudAPI(`/api/datapoint/${encodeURIComponent(booleanPoint.dataPointId)}/enabled`, {
    method: "PUT",
    body: { enabled: false },
  });
  assert.equal(disabled.currentValue.quality, "NO_DATA");
  assert.equal(disabled.currentValue.value, null);
  const resetRevision = disabled.currentValue.revision;
  await delay(1500);
  const noRawWhileDisabled = await cloudAPI(`/api/datapoint/${encodeURIComponent(booleanPoint.dataPointId)}`);
  assert.equal(noRawWhileDisabled.currentValue.revision, resetRevision, "disabled DataPoint changed without a new enable");

  const enabled = await cloudAPI(`/api/datapoint/${encodeURIComponent(booleanPoint.dataPointId)}/enabled`, {
    method: "PUT",
    body: { enabled: true },
  });
  assert.equal(enabled.enabled, true);
  const recovered = await waitForGood(booleanPoint.dataPointId, enabled.currentValue.revision);
  assert.ok(recovered.currentValue.revision > enabled.currentValue.revision, "enable did not wait for a new raw observation");
  return recovered;
}

function publishUnknownRawProbe() {
  const project = process.env.M4_MQTT_COMPOSE_PROJECT;
  if (!project) return false;
  const container = process.env.M4_MQTT_CONTAINER ?? `${project}-mqtt-1`;
  const port = process.env.M4_MQTT_CONTAINER_PORT ?? "1883";
  const topicPrefix = process.env.M4_MQTT_TOPIC_PREFIX ?? "edge";
  const edgeID = process.env.M4_UNKNOWN_EDGE ?? "m4-unknown-edge";
  const sourceDeviceID = process.env.M4_UNKNOWN_SOURCE_DEVICE ?? `m4-unknown-device-${Date.now()}`;
  const payload = JSON.stringify({
    schema: "raw-register-snapshot/v1",
    messageId: `m4-unknown-raw-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    edgeId: edgeID,
    deviceId: sourceDeviceID,
    timestamp: new Date().toISOString(),
    data: { communicationStatus: "ONLINE", blocks: [] },
  });
  const result = spawnSync("docker", [
    "exec", container, "mosquitto_pub", "-V", "mqttv5", "-h", "127.0.0.1", "-p", port,
    "-u", process.env.M4_MQTT_COLLECTOR_USER ?? "edge_collector",
    "-P", process.env.M4_MQTT_COLLECTOR_PASSWORD ?? "edge-collector-mqtt-dev",
    "-t", `${topicPrefix}/${edgeID}/device/${sourceDeviceID}/raw`, "-q", "0", "-m", payload,
  ], { encoding: "utf8" });
  assert.equal(result.status, 0, `unknown raw probe failed: ${result.stderr ?? result.stdout}`);
  return true;
}

function queryPostgres(container, user, password, database, query) {
  const result = spawnSync("docker", [
    "exec", container, "env", `PGPASSWORD=${password}`, "psql", "-h", "127.0.0.1", "-U", user,
    "-d", database, "-At", "-c", query,
  ], { encoding: "utf8" });
  assert.equal(result.status, 0, `PostgreSQL schema probe failed: ${result.stderr ?? result.stdout}`);
  return result.stdout.trim();
}

async function assertSchemaAndUnknownRaw() {
  const project = process.env.M4_MQTT_COMPOSE_PROJECT;
  if (!project) {
    console.log("M4 optional MQTT/schema probes skipped (set M4_MQTT_COMPOSE_PROJECT for them)");
    return;
  }
  const before = await cloudAPI("/api/datapoint/page?page=1&pageSize=500");
  assert.equal(publishUnknownRawProbe(), true);
  await delay(1500);
  const after = await cloudAPI("/api/datapoint/page?page=1&pageSize=500");
  assert.equal(after.total, before.total, "unknown raw changed the DataPoint count");

  const container = process.env.M4_POSTGRES_CONTAINER ?? `${project}-postgres-1`;
  const user = process.env.M4_POSTGRES_USER ?? "edge_platform";
  const password = process.env.M4_POSTGRES_PASSWORD ?? "edge-platform-db-dev";
  const database = process.env.M4_POSTGRES_DATABASE ?? "edge_platform";
  const tables = queryPostgres(
    container,
    user,
    password,
    database,
    "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name",
  ).split("\n").filter(Boolean);
  for (const required of ["data_point", "source_mapping", "current_value"]) {
    assert.ok(tables.includes(required), `M4 table ${required} is missing`);
  }
  const forbidden = tables.filter((table) => /history|websocket|command|hmi|staging|payload|pending_message|device_event/i.test(table));
  assert.deepEqual(forbidden, [], `M4 introduced forbidden persistence tables: ${forbidden.join(", ")}`);
}

const numberPoint = await cloudAPI("/api/datapoint", {
  method: "POST",
  body: {
    deviceId: deviceID,
    pointKey: numberPointKey,
    name: "M4 Current A",
    valueType: "NUMBER",
    unit: "A",
    precision: 1,
    mapping: numberMapping(),
  },
});
const booleanPoint = await cloudAPI("/api/datapoint", {
  method: "POST",
  body: {
    deviceId: deviceID,
    pointKey: booleanPointKey,
    name: "M4 Breaker Closed",
    valueType: "BOOLEAN",
    mapping: booleanMapping(),
  },
});
assert.ok(numberPoint.dataPointId);
assert.ok(booleanPoint.dataPointId);

const firstNumber = await waitForGood(numberPoint.dataPointId);
const firstBoolean = await waitForGood(booleanPoint.dataPointId);
assert.equal(firstNumber.currentValue.quality, "GOOD");
assert.equal(firstBoolean.currentValue.quality, "GOOD");
assert.equal(typeof firstNumber.currentValue.value, "number");
assert.equal(typeof firstBoolean.currentValue.value, "boolean");
await assertRESTFilters(firstNumber);
await assertRESTFilters(firstBoolean);

const metadataNumber = await assertMetadataUpdatePreservesCurrent(numberPoint.dataPointId);
const { recoveredNumber, recoveredBoolean } = await assertRealFailureAndRecovery(numberPoint.dataPointId, booleanPoint.dataPointId);
const remappedNumber = await assertMappingResetAndRecovery(metadataNumber);
const reenabledBoolean = await assertDisableEnableRequiresNewRaw(recoveredBoolean);
await assertSchemaAndUnknownRaw();

console.log(JSON.stringify({
  number: remappedNumber.currentValue,
  boolean: reenabledBoolean.currentValue,
  acceptance: [
    "real Collector raw -> Cloud CurrentValue GOOD",
    "metadata update preserves CurrentValue",
    "real acquisition failure produces BAD while retaining the last value",
    "real acquisition recovery produces GOOD",
    "mapping change and disable/enable require a new raw observation",
  ],
  initialAfterRecovery: { number: recoveredNumber.currentValue, boolean: recoveredBoolean.currentValue },
}, null, 2));
