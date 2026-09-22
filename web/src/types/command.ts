import type { ApiPageRequest } from "@/types/api";

export type CommandStatus =
  | "PENDING"
  | "ACCEPTED"
  | "REJECTED"
  | "EXPIRED"
  | "SUCCEEDED"
  | "FAILED";

export type CommandRecord = {
  commandId: string;
  deviceId: string;
  edgeId: string;
  sourceDeviceId: string;
  name: string;
  args: Record<string, unknown>;
  requestedBy: number;
  issuedAt: string;
  expiresAt: string;
  status: CommandStatus;
  deliveryExpiredAt?: string | null;
  edgeReceivedAt?: string | null;
  startedAt?: string | null;
  completedAt?: string | null;
  resultReceivedAt?: string | null;
  result?: unknown;
  errorType?: string | null;
  errorMessage?: string | null;
};

export type CommandPageQuery = ApiPageRequest & {
  commandId?: string;
  deviceId?: string;
  name?: string;
  status?: CommandStatus;
  requestedBy?: number;
};

export type CreateCommandRequest = {
  commandId: string;
  deviceId: string;
  name: string;
  args: Record<string, unknown>;
  ttlSeconds: number;
};
