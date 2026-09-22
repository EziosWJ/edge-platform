import type {
  DataPointQuality,
  DataPointValueType,
} from "@/types/datapoint";

export type RealtimePoint = {
  deviceId: string;
  pointKey: string;
};

export type RealtimeCurrentValue = RealtimePoint & {
  dataPointId: string;
  valueType: DataPointValueType;
  value: number | boolean | null;
  quality: DataPointQuality;
  sourceTimestamp: string | null;
  observedAt: string | null;
  revision: number;
};

export type RealtimeTicketResponse = {
  ticket: string;
  expiresAt?: string;
};

export type RealtimeSubscribeMessage = {
  type: "subscribe" | "unsubscribe";
  requestId: string;
  points: RealtimePoint[];
};

export type RealtimeServerMessage =
  | ({ type: "subscribed"; requestId: string } & Partial<{
      point: RealtimePoint;
    }>)
  | ({ type: "unsubscribed"; requestId: string; point: RealtimePoint })
  | ({ type: "snapshot" | "update" } & RealtimeCurrentValue)
  | {
      type: "error";
      requestId?: string;
      code?: string;
      message?: string;
    };

export type RealtimeConnectionStatus =
  | "idle"
  | "requesting-ticket"
  | "connecting"
  | "subscribing"
  | "live"
  | "reconnecting"
  | "error"
  | "closed";
