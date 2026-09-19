import { z } from "zod";
import type { TreeCheckNode } from "@/components/common/tree-check-list";
import type {
  ApiStatus,
  RoleListRecord,
  SystemMenuTreeRecord,
} from "@/types";

export type RoleFilterState = {
  roleName: string;
  roleCode: string;
  status: "all" | ApiStatus;
};

export type RoleFormMode = "create" | "edit";

export const DEFAULT_ROLE_FILTERS: RoleFilterState = {
  roleName: "",
  roleCode: "",
  status: "all",
};

const emptyToUndefined = (value: unknown) =>
  value === "" || value === null ? undefined : value;

export const roleFormSchema = z.object({
  roleName: z
    .string()
    .trim()
    .min(1, "角色名称不能为空")
    .max(64, "角色名称不能超过 64 个字符"),
  roleCode: z
    .string()
    .trim()
    .min(1, "角色编码不能为空")
    .max(64, "角色编码不能超过 64 个字符"),
  status: z.coerce.number().pipe(z.union([z.literal(0), z.literal(1)])),
  sortOrder: z.coerce.number().int("排序必须是整数").min(0, "排序不能小于 0"),
  remark: z.preprocess(
    emptyToUndefined,
    z.string().trim().max(200, "备注不能超过 200 个字符").optional(),
  ),
});

export type RoleFormValues = z.infer<typeof roleFormSchema>;

export function buildQuery(
  filters: RoleFilterState,
  page: number,
  pageSize: number,
) {
  return {
    page,
    pageSize,
    roleName: filters.roleName.trim() || undefined,
    roleCode: filters.roleCode.trim() || undefined,
    status: filters.status === "all" ? undefined : filters.status,
  };
}

export function buildRolePayload(values: RoleFormValues) {
  return {
    roleName: values.roleName.trim(),
    roleCode: values.roleCode.trim(),
    status: values.status,
    sortOrder: values.sortOrder,
    remark: values.remark?.trim(),
  };
}

export function toFormValues(role?: RoleListRecord): RoleFormValues {
  return {
    roleName: role?.roleName ?? "",
    roleCode: role?.roleCode ?? "",
    status: role?.status === 0 ? 0 : 1,
    sortOrder: role?.sortOrder ?? 0,
    remark: role?.remark ?? "",
  };
}

function menuTypeLabel(type: SystemMenuTreeRecord["menuType"]) {
  const labels: Record<SystemMenuTreeRecord["menuType"], string> = {
    DIR: "目录",
    MENU: "菜单",
    LINK: "外链",
  };

  return labels[type];
}

export function menuTreeToCheckNodes(
  items: SystemMenuTreeRecord[],
): TreeCheckNode[] {
  return items.map((item) => ({
    id: item.id,
    label: `${item.menuName} / ${menuTypeLabel(item.menuType)}${
      item.permissionCode ? ` / ${item.permissionCode}` : ""
    }`,
    disabled: item.status === 0,
    children: item.children?.length
      ? menuTreeToCheckNodes(item.children)
      : undefined,
  }));
}

export function normalizeCheckedIds(ids: Array<string | number>) {
  return ids
    .map((id) => Number(id))
    .filter((id) => Number.isInteger(id) && id > 0);
}
