import { http } from "@/lib/http";
import type { ApiPageResult } from "@/types/api";
import type {
  AssignableRole,
  UserAssignRolesRequest,
  UserBatchDeleteRequest,
  UserCreateRequest,
  UserListQuery,
  UserRecord,
  UserStatusRequest,
  UserUpdateRequest,
} from "@/types/user";

const USER_BASE_PATH = "/api/system/user";

export function getUserPage(query: UserListQuery) {
  return http.get<ApiPageResult<UserRecord>>(`${USER_BASE_PATH}/page`, {
    query,
  });
}

export function getUserDetail(id: number) {
  return http.get<UserRecord>(`${USER_BASE_PATH}/${id}`);
}

export function createUser(data: UserCreateRequest) {
  return http.post<void>(USER_BASE_PATH, data);
}

export function updateUser(id: number, data: UserUpdateRequest) {
  return http.put<void>(`${USER_BASE_PATH}/${id}`, data);
}

export function deleteUser(id: number) {
  return http.delete<void>(`${USER_BASE_PATH}/${id}`);
}

export function batchDeleteUsers(data: UserBatchDeleteRequest) {
  return http.post<void>(`${USER_BASE_PATH}/batch-delete`, data);
}

export function updateUserStatus(id: number, data: UserStatusRequest) {
  return http.patch<void>(`${USER_BASE_PATH}/${id}/status`, data);
}

export function assignUserRoles(id: number, data: UserAssignRolesRequest) {
  return http.put<void>(`${USER_BASE_PATH}/${id}/roles`, data);
}

export function resetUserPassword(id: number) {
  return http.put<unknown>(`${USER_BASE_PATH}/${id}/reset-password`).then((data) => {
    if (typeof data === "string") return data;
    if (data && typeof data === "object" && "password" in data) {
      const password = (data as { password?: unknown }).password;
      if (typeof password === "string") return password;
      if (password !== null && password !== undefined) return String(password);
    }

    return "";
  });
}

export function getAssignableRoles() {
  return http.get<AssignableRole[]>("/api/system/role/options");
}
