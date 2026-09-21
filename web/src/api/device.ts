import type { ApiPageResult } from "@/types/api";
import type { DevicePageQuery, DeviceRecord } from "@/types/device";
import { http } from "@/lib/http";

const DEVICE_BASE_PATH = "/api/device";

export function getDevicePage(query: DevicePageQuery) {
  return http.get<ApiPageResult<DeviceRecord>>(`${DEVICE_BASE_PATH}/page`, {
    query,
  });
}

export function getDeviceDetail(deviceId: string) {
  return http.get<DeviceRecord>(
    `${DEVICE_BASE_PATH}/${encodeURIComponent(deviceId)}`,
  );
}
