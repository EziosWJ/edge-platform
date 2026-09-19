import { http } from "@/lib/http";
import type { ApiPageResult } from "@/types/api";
import type {
  DeptBatchDeleteRequest,
  DeptCreateRequest,
  DeptListQuery,
  DeptOption,
  DeptRecord,
  DeptStatusRequest,
  DeptUpdateRequest,
} from "@/types/dept";
import type { TreeSelectNode } from "@/components/common/tree-select";

const DEPT_BASE_PATH = "/api/system/dept";

export function getDeptTree() {
  return http.get<DeptRecord[]>(`${DEPT_BASE_PATH}/tree`);
}

export function getDeptOptions() {
  return http.get<DeptOption[]>(`${DEPT_BASE_PATH}/options`);
}

export function getDeptPage(query: DeptListQuery) {
  return http.get<ApiPageResult<DeptRecord>>(`${DEPT_BASE_PATH}/page`, {
    query,
  });
}

export function getDeptDetail(id: number) {
  return http.get<DeptRecord>(`${DEPT_BASE_PATH}/${id}`);
}

export function createDept(data: DeptCreateRequest) {
  return http.post<void>(DEPT_BASE_PATH, data);
}

export function updateDept(id: number, data: DeptUpdateRequest) {
  return http.put<void>(`${DEPT_BASE_PATH}/${id}`, data);
}

export function deleteDept(id: number) {
  return http.delete<void>(`${DEPT_BASE_PATH}/${id}`);
}

export function batchDeleteDepts(data: DeptBatchDeleteRequest) {
  return http.post<void>(`${DEPT_BASE_PATH}/batch-delete`, data);
}

export function updateDeptStatus(id: number, data: DeptStatusRequest) {
  return http.patch<void>(`${DEPT_BASE_PATH}/${id}/status`, data);
}

export function deptOptionsToTreeSelectNodes(
  options: DeptOption[],
  disabledIds: Array<number | string> = [],
): TreeSelectNode[] {
  const disabledIdSet = new Set(disabledIds.map(String));

  return options.map((option) => ({
    id: option.id,
    label: option.deptName,
    disabled: option.disabled || disabledIdSet.has(String(option.id)),
    children: option.children?.length
      ? deptOptionsToTreeSelectNodes(option.children, disabledIds)
      : undefined,
  }));
}
