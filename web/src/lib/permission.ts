import { mockCurrentUserPermissionCodes } from "@/mocks/rbac";

export function hasPermission(permissionCode: string): boolean {
  return mockCurrentUserPermissionCodes.includes(permissionCode);
}

export function hasAnyPermission(permissionCodes: string[]): boolean {
  return permissionCodes.some((code) => hasPermission(code));
}

export function hasAllPermissions(permissionCodes: string[]): boolean {
  return permissionCodes.every((code) => hasPermission(code));
}

