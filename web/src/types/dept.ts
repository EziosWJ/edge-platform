import type { ApiBuiltinFlag, ApiPageRequest, ApiStatus } from "./api";

export type DeptRecord = {
  id: number;
  parentId: number;
  deptName: string;
  deptCode: string;
  leader?: string | null;
  phone?: string | null;
  email?: string | null;
  sortOrder: number;
  status: ApiStatus;
  isBuiltin: ApiBuiltinFlag;
  remark?: string | null;
  createTime?: string | null;
  updateTime?: string | null;
  children?: DeptRecord[];
};

export type DeptOption = {
  id: number;
  parentId: number;
  deptName: string;
  deptCode?: string;
  disabled?: boolean;
  children?: DeptOption[];
};

export type DeptListQuery = Partial<ApiPageRequest> & {
  deptName?: string;
  deptCode?: string;
  status?: ApiStatus;
};

export type DeptCreateRequest = {
  parentId: number;
  deptName: string;
  deptCode: string;
  leader?: string;
  phone?: string;
  email?: string;
  sortOrder: number;
  status: ApiStatus;
  remark?: string;
};

export type DeptUpdateRequest = DeptCreateRequest;

export type DeptStatusRequest = {
  status: ApiStatus;
};

export type DeptBatchDeleteRequest = {
  ids: number[];
};
