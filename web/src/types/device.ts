import type { ApiPageRequest } from "@/types/api";

export type DeviceCommunicationStatus =
  | "INITIAL"
  | "ONLINE"
  | "DEGRADED"
  | "OFFLINE";

export type DeviceRecord = {
  deviceId: string;
  edgeId: string;
  sourceDeviceId: string;
  communicationStatus: DeviceCommunicationStatus;
  registeredAt: string;
  lastSeenAt: string;
  lastAttemptAt: string | null;
  lastSuccessAt: string | null;
  communicationError: string | null;
};

export type DevicePageQuery = ApiPageRequest & {
  edgeId?: string;
  deviceId?: string;
  sourceDeviceId?: string;
  status?: DeviceCommunicationStatus;
};
