import { createRealtimeTicket } from "@/api/realtime";
import { buildApiUrl } from "@/lib/http";
import type {
  RealtimeConnectionStatus,
  RealtimeCurrentValue,
  RealtimePoint,
  RealtimeServerMessage,
  RealtimeSubscribeMessage,
} from "@/types/realtime";

export type RealtimeSocket = {
  onopen: ((event: Event) => void) | null;
  onmessage: ((event: MessageEvent) => void) | null;
  onerror: ((event: Event) => void) | null;
  onclose: ((event: CloseEvent) => void) | null;
  send: (data: string) => void;
  close: (code?: number, reason?: string) => void;
};

type RealtimeTimer = ReturnType<typeof globalThis.setTimeout>;
type RealtimeSetTimeout = (handler: () => void, timeout: number) => RealtimeTimer;
type RealtimeClearTimeout = (timer: RealtimeTimer) => void;

export type RealtimeClientOptions = {
  points: RealtimePoint[];
  requestTicket?: typeof createRealtimeTicket;
  webSocketFactory?: (url: string) => RealtimeSocket;
  reconnect?: {
    initialDelayMs?: number;
    maxDelayMs?: number;
    jitterRatio?: number;
  };
  random?: () => number;
  setTimeout?: RealtimeSetTimeout;
  clearTimeout?: RealtimeClearTimeout;
  onValue?: (value: RealtimeCurrentValue) => void;
  onStatus?: (status: RealtimeConnectionStatus) => void;
  onError?: (error: Error) => void;
};

function createBrowserSocket(url: string): RealtimeSocket {
  return new WebSocket(url);
}

export function buildRealtimeWebSocketUrl(ticket: string) {
  const url = new URL(buildApiUrl("/api/realtime/ws"), window.location.href);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.search = "";
  url.searchParams.set("ticket", ticket);
  return url.toString();
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isPoint(value: unknown): value is RealtimePoint {
  return isRecord(value) && typeof value.deviceId === "string" && typeof value.pointKey === "string";
}

function isCurrentValue(value: unknown): value is RealtimeCurrentValue {
  if (!isRecord(value)) return false;
  if (
    typeof value.dataPointId !== "string" ||
    typeof value.deviceId !== "string" ||
    typeof value.pointKey !== "string" ||
    (value.valueType !== "NUMBER" && value.valueType !== "BOOLEAN") ||
    !["NO_DATA", "GOOD", "BAD"].includes(String(value.quality)) ||
    typeof value.revision !== "number" ||
    !Number.isInteger(value.revision) ||
    value.revision < 0
  ) return false;

  const validValue =
    value.value === null ||
    (value.valueType === "NUMBER" && typeof value.value === "number") ||
    (value.valueType === "BOOLEAN" && typeof value.value === "boolean");
  const validTimestamp = value.sourceTimestamp === null || typeof value.sourceTimestamp === "string";
  const validObservedAt = value.observedAt === null || typeof value.observedAt === "string";
  return validValue && validTimestamp && validObservedAt;
}

function parseServerMessage(data: unknown): RealtimeServerMessage | null {
  if (typeof data !== "string") return null;
  try {
    const parsed = JSON.parse(data) as unknown;
    if (!isRecord(parsed) || typeof parsed.type !== "string") return null;
    if (parsed.type === "snapshot" || parsed.type === "update") {
      return isCurrentValue(parsed) ? (parsed as RealtimeServerMessage) : null;
    }
    if (parsed.type === "subscribed" || parsed.type === "unsubscribed") {
      if (typeof parsed.requestId !== "string" || !isPoint(parsed.point)) return null;
      return parsed as RealtimeServerMessage;
    }
    if (parsed.type === "error") return parsed as RealtimeServerMessage;
  } catch {
    return null;
  }
  return null;
}

function pointKey(point: RealtimePoint) {
  return `${point.deviceId}\u0000${point.pointKey}`;
}

function uniquePoints(points: RealtimePoint[]) {
  const result: RealtimePoint[] = [];
  const seen = new Set<string>();
  for (const point of points) {
    const key = pointKey(point);
    if (seen.has(key)) continue;
    seen.add(key);
    result.push(point);
  }
  return result;
}

function pointSet(points: RealtimePoint[]) {
  return new Map(uniquePoints(points).map((point) => [pointKey(point), point]));
}

export class RealtimeClient {
  private static readonly defaultReconnectInitialDelayMs = 500;
  private static readonly defaultReconnectMaxDelayMs = 15_000;
  private static readonly defaultReconnectJitterRatio = 0.2;
  private desiredPoints: RealtimePoint[];
  private activePoints = new Map<string, RealtimePoint>();
  private readonly requestTicket: typeof createRealtimeTicket;
  private readonly webSocketFactory: (url: string) => RealtimeSocket;
  private readonly onValue?: (value: RealtimeCurrentValue) => void;
  private readonly onStatus?: (status: RealtimeConnectionStatus) => void;
  private readonly onError?: (error: Error) => void;
  private socket: RealtimeSocket | null = null;
  private socketOpen = false;
  private lifecycle = 0;
  private requestSequence = 0;
  private reconnectAttempt = 0;
  private reconnectTimer: RealtimeTimer | null = null;
  private manuallyDisconnected = false;
  private connectionStatus: RealtimeConnectionStatus = "idle";
  private readonly reconnectInitialDelayMs: number;
  private readonly reconnectMaxDelayMs: number;
  private readonly reconnectJitterRatio: number;
  private readonly random: () => number;
  private readonly setTimer: RealtimeSetTimeout;
  private readonly clearTimer: RealtimeClearTimeout;

  constructor(options: RealtimeClientOptions) {
    this.desiredPoints = uniquePoints(options.points);
    this.requestTicket = options.requestTicket ?? createRealtimeTicket;
    this.webSocketFactory = options.webSocketFactory ?? createBrowserSocket;
    this.reconnectInitialDelayMs = Math.max(0, options.reconnect?.initialDelayMs ?? RealtimeClient.defaultReconnectInitialDelayMs);
    this.reconnectMaxDelayMs = Math.max(this.reconnectInitialDelayMs, options.reconnect?.maxDelayMs ?? RealtimeClient.defaultReconnectMaxDelayMs);
    this.reconnectJitterRatio = Math.min(1, Math.max(0, options.reconnect?.jitterRatio ?? RealtimeClient.defaultReconnectJitterRatio));
    this.random = options.random ?? Math.random;
    this.setTimer = options.setTimeout ?? ((handler, timeout) => globalThis.setTimeout(handler, timeout));
    this.clearTimer = options.clearTimeout ?? ((timer) => globalThis.clearTimeout(timer));
    this.onValue = options.onValue;
    this.onStatus = options.onStatus;
    this.onError = options.onError;
  }

  setPoints(points: RealtimePoint[]) {
    this.desiredPoints = uniquePoints(points);
    if (this.desiredPoints.length === 0) {
      this.lifecycle += 1;
      this.cancelReconnect();
      const socket = this.socket;
      this.socket = null;
      this.socketOpen = false;
      this.activePoints.clear();
      if (socket) {
        socket.onopen = null;
        socket.onmessage = null;
        socket.onerror = null;
        socket.onclose = null;
        try {
          socket.close(1000, "realtime points cleared");
        } catch {
          // The browser may already have torn down the socket.
        }
      }
      this.setStatus("idle");
      return;
    }
    if (!this.socket || !this.socketOpen) {
      return;
    }
    const desired = pointSet(this.desiredPoints);
    const removed = [...this.activePoints.entries()]
      .filter(([key]) => !desired.has(key))
      .map(([, point]) => point);
    const added = [...desired.entries()]
      .filter(([key]) => !this.activePoints.has(key))
      .map(([, point]) => point);
    if (removed.length > 0) this.sendPoints("unsubscribe", removed);
    if (added.length > 0) this.sendPoints("subscribe", added);
    this.activePoints = desired;
  }

  async connect() {
    this.manuallyDisconnected = false;
    this.cancelReconnect();
    if (this.socket || this.connectionStatus === "requesting-ticket") return;
    if (this.desiredPoints.length === 0) {
      this.setStatus("idle");
      return;
    }
    const lifecycle = ++this.lifecycle;
    this.setStatus("requesting-ticket");
    try {
      const { ticket } = await this.requestTicket();
      if (lifecycle !== this.lifecycle) return;
      if (!ticket) throw new Error("实时连接 ticket 为空");
      const socket = this.webSocketFactory(buildRealtimeWebSocketUrl(ticket));
      this.socket = socket;
      this.socketOpen = false;
      this.activePoints.clear();
      this.setStatus("connecting");
      socket.onopen = () => {
        if (lifecycle !== this.lifecycle || this.socket !== socket) return;
        this.socketOpen = true;
        this.reconnectAttempt = 0;
        this.setStatus("subscribing");
        this.sendPoints("subscribe", this.desiredPoints);
        this.activePoints = pointSet(this.desiredPoints);
      };
      socket.onmessage = (event) => {
        if (lifecycle === this.lifecycle && this.socket === socket) this.handleMessage(parseServerMessage(event.data));
      };
      socket.onerror = () => {
        if (lifecycle !== this.lifecycle || this.socket !== socket) return;
        const error = new Error("实时 WebSocket 连接失败");
        this.setStatus("error");
        this.onError?.(error);
        this.detachSocket(socket);
        try {
          socket.close(1011, "realtime connection error");
        } catch {
          // The browser may already have torn down the socket.
        }
        this.scheduleReconnect(lifecycle);
      };
      socket.onclose = () => {
        if (lifecycle !== this.lifecycle || this.socket !== socket) return;
        this.detachSocket(socket);
        this.setStatus("closed");
        this.scheduleReconnect(lifecycle);
      };
    } catch (error) {
      if (lifecycle !== this.lifecycle) return;
      const normalized = error instanceof Error ? error : new Error("实时连接失败");
      this.setStatus("error");
      this.onError?.(normalized);
      this.scheduleReconnect(lifecycle);
      throw normalized;
    }
  }

  disconnect() {
    this.manuallyDisconnected = true;
    this.lifecycle += 1;
    this.cancelReconnect();
    const socket = this.socket;
    this.socket = null;
    this.socketOpen = false;
    this.activePoints.clear();
    if (!socket) {
      this.setStatus("closed");
      return;
    }
    socket.onopen = null;
    socket.onmessage = null;
    socket.onerror = null;
    socket.onclose = null;
    socket.close(1000, "realtime page lifecycle ended");
    this.setStatus("closed");
  }

  private sendPoints(type: RealtimeSubscribeMessage["type"], points: RealtimePoint[]) {
    if (!this.socket || points.length === 0) return;
    const message: RealtimeSubscribeMessage = { type, requestId: `realtime-${++this.requestSequence}`, points };
    this.socket.send(JSON.stringify(message));
  }

  private handleMessage(message: RealtimeServerMessage | null) {
    if (!message) return;
    if (message.type === "subscribed") {
      this.setStatus("live");
      return;
    }
    if (message.type === "unsubscribed") return;
    if (message.type === "error") {
      this.onError?.(new Error(message.message || "实时订阅失败"));
      return;
    }
    this.onValue?.(message);
  }

  private setStatus(status: RealtimeConnectionStatus) {
    this.connectionStatus = status;
    this.onStatus?.(status);
  }

  private detachSocket(socket: RealtimeSocket) {
    if (this.socket !== socket) return;
    this.socket = null;
    this.socketOpen = false;
    this.activePoints.clear();
  }

  private scheduleReconnect(lifecycle: number) {
    if (lifecycle !== this.lifecycle || this.manuallyDisconnected || this.desiredPoints.length === 0 || this.reconnectTimer || this.socket) return;
    const exponentialDelay = Math.min(
      this.reconnectMaxDelayMs,
      this.reconnectInitialDelayMs * 2 ** Math.min(this.reconnectAttempt, 30),
    );
    this.reconnectAttempt += 1;
    const jitter = exponentialDelay * this.reconnectJitterRatio;
    const delay = Math.max(0, exponentialDelay + (this.random() * 2 - 1) * jitter);
    this.setStatus("reconnecting");
    this.reconnectTimer = this.setTimer(() => {
      this.reconnectTimer = null;
      void this.connect().catch(() => undefined);
    }, delay);
  }

  private cancelReconnect() {
    if (this.reconnectTimer === null) return;
    this.clearTimer(this.reconnectTimer);
    this.reconnectTimer = null;
  }
}

// Compatibility adapter for callers that still need a single point client.
export class RealtimePointClient extends RealtimeClient {
  constructor(options: Omit<RealtimeClientOptions, "points"> & { point: RealtimePoint }) {
    super({ ...options, points: [options.point] });
  }
}
