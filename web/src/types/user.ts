import type { ApiBuiltinFlag, ApiPageRequest, ApiStatus } from "./api";

export type UserGender = "MALE" | "FEMALE" | "UNKNOWN";
export type UserStatus = "active" | "pending" | "disabled";

export type UserRoleBrief = {
  id: number;
  roleName: string;
  roleCode?: string;
};

export type UserRecord = {
  id: number;
  username?: string;
  nickname?: string;
  phone?: string | null;
  email?: string | null;
  avatar?: string | null;
  gender?: UserGender | null;
  status: ApiStatus | UserStatus;
  isBuiltin?: ApiBuiltinFlag;
  deptId?: number | null;
  deptName?: string | null;
  roles?: UserRoleBrief[];
  lastLoginTime?: string | null;
  createTime?: string | null;
  remark?: string | null;
  name: string;
  account: string;
  role?: string;
  department: string;
  lastLogin?: string;
};

export type UserListQuery = Partial<ApiPageRequest> & {
  keyword?: string;
  username?: string;
  nickname?: string;
  phone?: string;
  email?: string;
  status?: ApiStatus | UserStatus | "all";
  deptId?: number;
};

export type UserCreateRequest = {
  username: string;
  nickname: string;
  phone?: string;
  email?: string;
  gender?: UserGender;
  deptId?: number;
  status: ApiStatus;
  remark?: string;
};

export type UserUpdateRequest = Omit<UserCreateRequest, "username">;

export type UserStatusRequest = {
  status: ApiStatus;
};

export type UserAssignRolesRequest = {
  roleIds: number[];
};

export type UserBatchDeleteRequest = {
  ids: number[];
};

export type AssignableRole = {
  id: number;
  roleName: string;
  roleCode: string;
  status: ApiStatus;
};
