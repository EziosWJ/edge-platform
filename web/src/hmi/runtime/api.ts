import { http } from "@/lib/http";
import type { HmiDocument } from "@/hmi/model";
import { realtimePointKey } from "@/store/realtime-store";
import type { DataPointValueType } from "@/types/datapoint";

export type HmiRuntimeDataPoint = {
  dataPointId: string;
  deviceId: string;
  pointKey: string;
  name: string;
  valueType: DataPointValueType;
  unit: string | null;
  precision: number | null;
  enabled: boolean;
};

type HmiRuntimeBootstrapResponse = {
  canExecuteCommands: boolean;
  page: {
    pageId: string;
    name: string;
    description?: string | null;
  };
  version: {
    versionId: string;
    pageId: string;
    versionNo: number;
    sourceDraftRevision: number;
    document: HmiDocument;
    publishedBy?: number;
    publishedAt?: string;
  };
  dataPoints: HmiRuntimeDataPoint[];
};

export function getHmiRuntimeBootstrap(pageId: string, signal?: AbortSignal) {
  return http.get<HmiRuntimeBootstrapResponse>(
    `/api/hmi/page/${encodeURIComponent(pageId)}/runtime`,
    { signal },
  ).then(({ dataPoints, ...bootstrap }) => ({
    ...bootstrap,
    dataPoints: Object.fromEntries(
      dataPoints.map((dataPoint) => [realtimePointKey(dataPoint), dataPoint]),
    ),
  }));
}

export type HmiRuntimeBootstrap = Omit<HmiRuntimeBootstrapResponse, "dataPoints"> & {
  dataPoints: Record<string, HmiRuntimeDataPoint>;
};
