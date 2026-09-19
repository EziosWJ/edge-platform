import {
  filterMockPermissions,
  getMockPermissionTotal,
} from "@/mocks/rbac";
import { http } from "@/lib/http";
import type { ApiPageResult } from "@/types/api";
import type {
  PermissionQuery,
  PermissionRecord,
  RoleAssignMenusRequest,
  RoleBatchDeleteRequest,
  RoleCreateRequest,
  RoleDetailRecord,
  RoleListQuery,
  RoleListRecord,
  RoleStatusRequest,
  RoleUpdateRequest,
  SystemMenuBatchDeleteRequest,
  SystemMenuCreateRequest,
  SystemMenuListQuery,
  SystemMenuRecord,
  SystemMenuStatusRequest,
  SystemMenuTreeRecord,
  SystemMenuUpdateRequest,
} from "@/types";

function wait(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

const ROLE_BASE_PATH = "/api/system/role";
const MENU_BASE_PATH = "/api/system/menu";

export function getRolePage(query: RoleListQuery) {
  return http.get<ApiPageResult<RoleListRecord>>(`${ROLE_BASE_PATH}/page`, {
    query,
  });
}

export function getRoleDetail(id: number) {
  return http.get<RoleDetailRecord>(`${ROLE_BASE_PATH}/${id}`);
}

export function createRole(data: RoleCreateRequest) {
  return http.post<void>(ROLE_BASE_PATH, data);
}

export function updateRole(id: number, data: RoleUpdateRequest) {
  return http.put<void>(`${ROLE_BASE_PATH}/${id}`, data);
}

export function deleteRole(id: number) {
  return http.delete<void>(`${ROLE_BASE_PATH}/${id}`);
}

export function batchDeleteRoles(data: RoleBatchDeleteRequest) {
  return http.post<void>(`${ROLE_BASE_PATH}/batch-delete`, data);
}

export function updateRoleStatus(id: number, data: RoleStatusRequest) {
  return http.patch<void>(`${ROLE_BASE_PATH}/${id}/status`, data);
}

export function assignRoleMenus(id: number, data: RoleAssignMenusRequest) {
  return http.put<void>(`${ROLE_BASE_PATH}/${id}/menus`, data);
}

export function getSystemMenuTree() {
  return http.get<SystemMenuTreeRecord[]>(`${MENU_BASE_PATH}/tree`);
}

export function getMenuPage(query: SystemMenuListQuery) {
  return http.get<ApiPageResult<SystemMenuRecord>>(`${MENU_BASE_PATH}/page`, {
    query,
  });
}

export function getMenuDetail(id: number) {
  return http.get<SystemMenuRecord>(`${MENU_BASE_PATH}/${id}`);
}

export function createMenu(data: SystemMenuCreateRequest) {
  return http.post<void>(MENU_BASE_PATH, data);
}

export function updateMenu(id: number, data: SystemMenuUpdateRequest) {
  return http.put<void>(`${MENU_BASE_PATH}/${id}`, data);
}

export function deleteMenu(id: number) {
  return http.delete<void>(`${MENU_BASE_PATH}/${id}`);
}

export function batchDeleteMenus(data: SystemMenuBatchDeleteRequest) {
  return http.post<void>(`${MENU_BASE_PATH}/batch-delete`, data);
}

export function updateMenuStatus(id: number, data: SystemMenuStatusRequest) {
  return http.patch<void>(`${MENU_BASE_PATH}/${id}/status`, data);
}

export async function getPermissions(
  query: PermissionQuery = {},
): Promise<PermissionRecord[]> {
  await wait(300);
  return filterMockPermissions(query);
}

export function getPermissionTotal() {
  return getMockPermissionTotal();
}
