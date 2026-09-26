import type { ApiPageResult } from "@/types/api";
import { http } from "@/lib/http";
import type { HmiDocument } from "@/hmi/model";

const BASE = "/api/hmi/page";

export type HmiPageRecord = {
  pageId: string;
  name: string;
  description: string;
  draftRevision: number;
  publishedVersionId: string | null;
  createdAt: string;
  updatedAt: string;
};

export type HmiPageDetail = HmiPageRecord & { draftDocument: HmiDocument };
export type HmiPageQuery = { page: number; pageSize: number; name?: string };
export type HmiPageMetadataInput = { name: string; description: string };
export type SaveHmiDraftInput = { expectedDraftRevision: number; document: HmiDocument };
export type PublishHmiPageInput = { expectedDraftRevision: number };

export function getHmiPagePage(query: HmiPageQuery) {
  return http.get<ApiPageResult<HmiPageRecord>>(`${BASE}/page`, { query });
}

export function getHmiPage(pageId: string) {
  return http.get<HmiPageDetail>(`${BASE}/${encodeURIComponent(pageId)}`);
}

export function createHmiPage(body: HmiPageMetadataInput) {
  return http.post<HmiPageRecord>(BASE, body);
}

export function updateHmiPage(pageId: string, body: HmiPageMetadataInput) {
  return http.put<HmiPageRecord>(`${BASE}/${encodeURIComponent(pageId)}`, body);
}

export function saveHmiDraft(pageId: string, body: SaveHmiDraftInput) {
  return http.put<HmiPageDetail>(`${BASE}/${encodeURIComponent(pageId)}/draft`, body);
}

export function publishHmiPage(pageId: string, body: PublishHmiPageInput) {
  return http.post<HmiPageRecord & { versionNo: number; sourceDraftRevision: number }>(`${BASE}/${encodeURIComponent(pageId)}/publish`, body);
}
