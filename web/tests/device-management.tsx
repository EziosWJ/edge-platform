import { createRoot } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { DevicePage } from "@/pages/device";
import "@/styles/globals.css";

declare global {
  interface Window {
    deviceRequests: string[];
  }
}

const records = [
  {
    deviceId: "cloud-device-initial",
    edgeId: "edge-a",
    sourceDeviceId: "source-01",
    communicationStatus: "INITIAL",
    registeredAt: "2026-09-21T08:00:00Z",
    lastSeenAt: "2026-09-21T08:00:00Z",
    lastAttemptAt: null,
    lastSuccessAt: null,
    communicationError: null,
  },
  {
    deviceId: "cloud-device-online",
    edgeId: "edge-a",
    sourceDeviceId: "source-02",
    communicationStatus: "ONLINE",
    registeredAt: "2026-09-20T08:00:00Z",
    lastSeenAt: "2026-09-21T08:31:00Z",
    lastAttemptAt: "2026-09-21T08:30:59Z",
    lastSuccessAt: "2026-09-21T08:30:58Z",
    communicationError: null,
  },
  {
    deviceId: "cloud-device-degraded",
    edgeId: "edge-a",
    sourceDeviceId: "source-03",
    communicationStatus: "DEGRADED",
    registeredAt: "2026-09-19T08:00:00Z",
    lastSeenAt: "2026-09-21T08:32:00Z",
    lastAttemptAt: "2026-09-21T08:31:59Z",
    lastSuccessAt: "2026-09-21T08:30:58Z",
    communicationError: "partial read",
  },
  {
    deviceId: "cloud-device-offline",
    edgeId: "edge-a",
    sourceDeviceId: "source-04",
    communicationStatus: "OFFLINE",
    registeredAt: "2026-09-18T08:00:00Z",
    lastSeenAt: "2026-09-21T08:33:00Z",
    lastAttemptAt: "2026-09-21T08:32:59Z",
    lastSuccessAt: "2026-09-21T08:30:58Z",
    communicationError: "timeout",
  },
];

window.history.replaceState({}, "", "/tests/device-management.html?edgeId=edge-a");
window.deviceRequests = [];
window.fetch = async (input) => {
  const url = new URL(String(input), window.location.origin);
  window.deviceRequests.push(`${url.pathname}${url.search}`);

  if (url.pathname === "/api/device/page") {
    const page = Number(url.searchParams.get("page") ?? "1");
    const pageSize = Number(url.searchParams.get("pageSize") ?? "10");
    const status = url.searchParams.get("status");
    const edgeId = url.searchParams.get("edgeId");
    const deviceId = url.searchParams.get("deviceId");
    const sourceDeviceId = url.searchParams.get("sourceDeviceId");
    const filtered = records.filter(
      (record) =>
        (!status || record.communicationStatus === status) &&
        (!edgeId || record.edgeId === edgeId) &&
        (!deviceId || record.deviceId === deviceId) &&
        (!sourceDeviceId || record.sourceDeviceId === sourceDeviceId),
    );

    return new Response(
      JSON.stringify({
        code: 200,
        message: "success",
        data: { records: filtered, total: 21, page, pageSize },
      }),
      { headers: { "Content-Type": "application/json" } },
    );
  }

  if (url.pathname === "/api/device/cloud-device-degraded") {
    return new Response(
      JSON.stringify({ code: 200, message: "success", data: records[2] }),
      { headers: { "Content-Type": "application/json" } },
    );
  }

  return new Response(
    JSON.stringify({ code: 404, message: "Device not found", data: null }),
    { status: 404, headers: { "Content-Type": "application/json" } },
  );
};

createRoot(document.getElementById("root")!).render(
  <MemoryRouter>
    <DevicePage />
  </MemoryRouter>,
);
