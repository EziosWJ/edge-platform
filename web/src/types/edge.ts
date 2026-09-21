import type { ApiPageRequest } from "@/types/api";

export type EdgeStatus = "ONLINE" | "OFFLINE";

export type EdgeRecord = {
  edgeId: string;
  status: EdgeStatus;
  registeredAt: string;
  lastSeenAt: string;
};

export type EdgePageQuery = ApiPageRequest & {
  status?: EdgeStatus;
  edgeId?: string;
};
