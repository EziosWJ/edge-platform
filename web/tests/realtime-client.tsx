import { createRoot } from "react-dom/client";
import { DataPointPage } from "@/pages/datapoint";
import "@/styles/globals.css";

declare global {
  interface Window {
    realtimeRequests: Array<{ method: string; path: string; body: string | null }>;
    realtimeTest: {
      socket: FakeWebSocket | null;
      sockets: FakeWebSocket[];
    };
  }
}

const record = {
  dataPointId: "point-temperature",
  deviceId: "device-realtime",
  pointKey: "temperature",
  name: "柜内温度",
  valueType: "NUMBER",
  unit: "°C",
  precision: 1,
  enabled: true,
  currentValue: {
    value: 10,
    quality: "GOOD",
    sourceTimestamp: "2026-09-22T00:00:00Z",
    observedAt: "2026-09-22T00:00:01Z",
    revision: 1,
  },
};

const booleanRecord = {
  dataPointId: "point-enabled",
  deviceId: "device-realtime",
  pointKey: "pump_enabled",
  name: "泵状态",
  valueType: "BOOLEAN",
  unit: null,
  precision: null,
  enabled: true,
  currentValue: {
    value: false,
    quality: "GOOD",
    sourceTimestamp: "2026-09-22T00:00:00Z",
    observedAt: "2026-09-22T00:00:01Z",
    revision: 1,
  },
};

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  readonly url: string;
  readonly sent: string[] = [];
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
    window.realtimeTest.socket = this;
    queueMicrotask(() => this.onopen?.(new Event("open")));
  }

  send(data: string) {
    this.sent.push(data);
    const message = JSON.parse(data) as { type?: string; requestId?: string };
    if (message.type === "subscribe") {
      const request = JSON.parse(data) as { points?: Array<{ deviceId: string; pointKey: string }>; requestId?: string };
      for (const point of request.points ?? []) {
        queueMicrotask(() => this.emit({ type: "subscribed", requestId: message.requestId, point }));
      }
    }
    if (message.type === "unsubscribe") {
      const request = JSON.parse(data) as { points?: Array<{ deviceId: string; pointKey: string }>; requestId?: string };
      for (const point of request.points ?? []) {
        queueMicrotask(() => this.emit({ type: "unsubscribed", requestId: message.requestId, point }));
      }
    }
  }

  close() {}

  serverClose(code = 1013, reason = "server restart") {
    this.onclose?.({ code, reason, wasClean: false } as CloseEvent);
  }

  emit(message: unknown) {
    this.onmessage?.({ data: JSON.stringify(message) } as MessageEvent);
  }
}

window.realtimeRequests = [];
window.realtimeTest = { socket: null, sockets: FakeWebSocket.instances };
window.WebSocket = FakeWebSocket as unknown as typeof WebSocket;
window.fetch = async (input, init) => {
  const url = new URL(String(input), window.location.origin);
  window.realtimeRequests.push({
    method: init?.method ?? "GET",
    path: `${url.pathname}${url.search}`,
    body: typeof init?.body === "string" ? init.body : null,
  });

  if (url.pathname === "/api/datapoint/page") {
    const records = url.searchParams.get("pointKey") === "pump_enabled" ? [booleanRecord] : [record, booleanRecord];
    return new Response(
      JSON.stringify({
        code: 200,
        message: "success",
        data: { records, total: records.length, page: 1, pageSize: 500 },
      }),
      { headers: { "Content-Type": "application/json" } },
    );
  }

  if (url.pathname === "/api/realtime/ticket") {
    return new Response(
      JSON.stringify({
        code: 200,
        message: "success",
        data: { ticket: "short-lived-ticket" },
      }),
      { headers: { "Content-Type": "application/json" } },
    );
  }

  return new Response(
    JSON.stringify({ code: 404, message: "Not found", data: null }),
    { status: 404, headers: { "Content-Type": "application/json" } },
  );
};

createRoot(document.getElementById("root")!).render(<DataPointPage />);
