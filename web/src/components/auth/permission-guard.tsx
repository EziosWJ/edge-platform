import type { PropsWithChildren, ReactNode } from "react";
import {
  hasAllPermissions,
  hasAnyPermission,
  hasPermission,
} from "@/lib/permission";

type PermissionGuardProps = PropsWithChildren<{
  permissionCode?: string;
  anyPermissions?: string[];
  allPermissions?: string[];
  fallback?: ReactNode;
}>;

export function PermissionGuard({
  permissionCode,
  anyPermissions,
  allPermissions,
  fallback = null,
  children,
}: PermissionGuardProps) {
  const allowed =
    (permissionCode ? hasPermission(permissionCode) : true) &&
    (anyPermissions ? hasAnyPermission(anyPermissions) : true) &&
    (allPermissions ? hasAllPermissions(allPermissions) : true);

  if (!allowed) {
    return fallback ?? null;
  }

  return children;
}

