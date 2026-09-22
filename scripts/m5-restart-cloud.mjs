/*
 * Restart only the Cloud binary started by the retained M3 acceptance.
 *
 * M3 deliberately keeps the Cloud/Collector processes outside this process so
 * M5 can exercise a real Cloud restart without inventing a deployment
 * manager. The hook discovers the exact edge-platform-api process by its
 * executable and configured HTTP port, reuses its environment and cwd, then
 * waits for the same health endpoint.
 */
import { readdir, readFile, readlink } from "node:fs/promises";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";

if (!process.env.M5_CLOUD_URL) {
  throw new Error("M5_CLOUD_URL is required");
}

const cloudURL = new URL(process.env.M5_CLOUD_URL);
const expectedPort = cloudURL.port || (cloudURL.protocol === "https:" ? "443" : "80");
const healthURL = `${cloudURL.toString().replace(/\/$/, "")}/health`;

function log(message) {
  process.stdout.write(`[m5-restart-cloud] ${message}\n`);
}

async function processIDs() {
  return (await readdir("/proc"))
    .filter((value) => /^\d+$/.test(value))
    .map(Number);
}

async function readProcess(pid) {
  try {
    const commandLine = (await readFile(`/proc/${pid}/cmdline`)).toString().split("\0").filter(Boolean);
    const executable = commandLine[0] ?? "";
    if (!executable.endsWith("/edge-platform-api") && executable !== "edge-platform-api") return null;
    const environment = Object.fromEntries(
      (await readFile(`/proc/${pid}/environ`)).toString().split("\0").filter(Boolean).map((entry) => {
        const separator = entry.indexOf("=");
        return separator < 0 ? [entry, ""] : [entry.slice(0, separator), entry.slice(separator + 1)];
      }),
    );
    const address = environment.APP_HTTP__ADDRESS ?? "";
    if (!address.endsWith(`:${expectedPort}`) && expectedPort !== "80" && expectedPort !== "443") return null;
    return { pid, executable, cwd: await readlink(`/proc/${pid}/cwd`), environment };
  } catch {
    return null;
  }
}

async function findCloud() {
  const candidates = [];
  for (const pid of await processIDs()) {
    const candidate = await readProcess(pid);
    if (candidate) candidates.push(candidate);
  }
  if (candidates.length !== 1) {
    throw new Error(`expected exactly one Cloud edge-platform-api on port ${expectedPort}, found ${candidates.length}`);
  }
  return candidates[0];
}

async function isHealthy() {
  try {
    const response = await fetch(healthURL, { signal: AbortSignal.timeout(1000) });
    return response.ok;
  } catch {
    return false;
  }
}

async function waitForHealth(expected, timeout = 30000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if ((await isHealthy()) === expected) return;
    await delay(100);
  }
  throw new Error(`Cloud health did not become ${expected ? "ready" : "down"}: ${healthURL}`);
}

const cloud = await findCloud();
log(`stopping Cloud pid ${cloud.pid} (${cloud.executable})`);
process.kill(cloud.pid, "SIGTERM");
await waitForHealth(false);

const child = spawn(cloud.executable, [], {
  cwd: cloud.cwd,
  detached: true,
  env: cloud.environment,
  stdio: "ignore",
});
child.unref();
log(`started Cloud pid ${child.pid}; waiting for ${healthURL}`);
await waitForHealth(true);
log("Cloud restart completed");
