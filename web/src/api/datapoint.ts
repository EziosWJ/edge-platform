import type { ApiPageResult } from "@/types/api";
import type { DataPointPageQuery, DataPointRecord } from "@/types/datapoint";
import { http } from "@/lib/http";

const BASE = "/api/datapoint";
export function getDataPointPage(query: DataPointPageQuery) { return http.get<ApiPageResult<DataPointRecord>>(`${BASE}/page`, { query }); }
export function getDataPointDetail(id: string) { return http.get<DataPointRecord>(`${BASE}/${encodeURIComponent(id)}`); }
export function createDataPoint(body: unknown) { return http.post<DataPointRecord>(BASE, body); }
export function updateDataPoint(id: string, body: unknown) { return http.put<DataPointRecord>(`${BASE}/${encodeURIComponent(id)}`, body); }
export function setDataPointEnabled(id: string, enabled: boolean) { return http.put<DataPointRecord>(`${BASE}/${encodeURIComponent(id)}/enabled`, { enabled }); }
