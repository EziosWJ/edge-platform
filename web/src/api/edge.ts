import { http } from "@/lib/http";
import type { ApiPageResult } from "@/types/api";
import type { EdgePageQuery, EdgeRecord } from "@/types/edge";

const EDGE_BASE_PATH = "/api/edge";

export function getEdgePage(query: EdgePageQuery) {
  return http.get<ApiPageResult<EdgeRecord>>(`${EDGE_BASE_PATH}/page`, {
    query,
  });
}

export function getEdgeDetail(edgeId: string) {
  return http.get<EdgeRecord>(
    `${EDGE_BASE_PATH}/${encodeURIComponent(edgeId)}`,
  );
}
