import { z } from "zod";
import type { TreeSelectNode } from "@/components/common/tree-select";
import type {
  ApiStatus,
  SystemMenuRecord,
  SystemMenuTreeRecord,
  SystemMenuType,
} from "@/types";

export type MenuFilterState = {
  menuName: string;
  menuType: "all" | SystemMenuType;
  status: "all" | ApiStatus;
  visible: "all" | ApiStatus;
};

export type MenuRow = SystemMenuRecord & {
  level: number;
  hasChildren: boolean;
};

export type MenuFormMode = "create" | "edit";

export const DEFAULT_MENU_FILTERS: MenuFilterState = {
  menuName: "",
  menuType: "all",
  status: "all",
  visible: "all",
};

export const typeLabelMap: Record<SystemMenuType, string> = {
  DIR: "目录",
  MENU: "菜单",
  LINK: "外链",
};

const emptyToUndefined = (value: unknown) =>
  value === "" || value === null ? undefined : value;

export const menuFormSchema = z
  .object({
    parentId: z.coerce.number().int().min(0),
    menuName: z
      .string()
      .trim()
      .min(1, "菜单名称不能为空")
      .max(64, "菜单名称不能超过 64 个字符"),
    menuType: z.union([z.literal("DIR"), z.literal("MENU"), z.literal("LINK")]),
    path: z.preprocess(
      emptyToUndefined,
      z.string().trim().max(128, "路由路径不能超过 128 个字符").optional(),
    ),
    component: z.preprocess(
      emptyToUndefined,
      z.string().trim().max(128, "组件路径不能超过 128 个字符").optional(),
    ),
    icon: z.preprocess(
      emptyToUndefined,
      z.string().trim().max(64, "图标标识不能超过 64 个字符").optional(),
    ),
    permissionCode: z.preprocess(
      emptyToUndefined,
      z.string().trim().max(128, "权限标识不能超过 128 个字符").optional(),
    ),
    externalUrl: z.preprocess(
      emptyToUndefined,
      z.string().trim().max(255, "外链地址不能超过 255 个字符").optional(),
    ),
    sortOrder: z.coerce.number().int("排序必须是整数").min(0, "排序不能小于 0"),
    visible: z.coerce.number().pipe(z.union([z.literal(0), z.literal(1)])),
    status: z.coerce.number().pipe(z.union([z.literal(0), z.literal(1)])),
    remark: z.preprocess(
      emptyToUndefined,
      z.string().trim().max(200, "备注不能超过 200 个字符").optional(),
    ),
  })
  .superRefine((values, ctx) => {
    if (values.menuType !== "LINK" && !values.path) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["path"],
        message: "目录和菜单需要填写路由路径",
      });
    }

    if (values.menuType === "LINK" && !values.externalUrl) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["externalUrl"],
        message: "外链菜单需要填写外链地址",
      });
    }
  });

export type MenuFormValues = z.infer<typeof menuFormSchema>;

export function buildQuery(
  filters: MenuFilterState,
  page: number,
  pageSize: number,
) {
  return {
    page,
    pageSize,
    menuName: filters.menuName.trim() || undefined,
    menuType: filters.menuType === "all" ? undefined : filters.menuType,
    status: filters.status === "all" ? undefined : filters.status,
    visible: filters.visible === "all" ? undefined : filters.visible,
  };
}

function compactText(value?: string | null) {
  const text = value?.trim();
  return text || undefined;
}

export function buildMenuPayload(values: MenuFormValues) {
  return {
    parentId: values.parentId,
    menuName: values.menuName.trim(),
    menuType: values.menuType,
    path: compactText(values.path),
    component: values.menuType === "LINK" ? undefined : compactText(values.component),
    icon: compactText(values.icon),
    permissionCode: compactText(values.permissionCode),
    sortOrder: values.sortOrder,
    visible: values.visible,
    status: values.status,
    externalUrl:
      values.menuType === "LINK" ? compactText(values.externalUrl) : undefined,
    remark: compactText(values.remark),
  };
}

export function toFormValues(
  menu?: SystemMenuRecord,
  parentId = 0,
): MenuFormValues {
  return {
    parentId: menu?.parentId ?? parentId,
    menuName: menu?.menuName ?? "",
    menuType: menu?.menuType ?? "MENU",
    path: menu?.path ?? "",
    component: menu?.component ?? "",
    icon: menu?.icon ?? "",
    permissionCode: menu?.permissionCode ?? "",
    externalUrl: menu?.externalUrl ?? "",
    sortOrder: menu?.sortOrder ?? 0,
    visible: menu?.visible === 0 ? 0 : 1,
    status: menu?.status === 0 ? 0 : 1,
    remark: menu?.remark ?? "",
  };
}

export function flattenMenuTree(
  items: SystemMenuRecord[],
  level = 0,
): MenuRow[] {
  return items.flatMap((item) => {
    const hasChildren = Boolean(item.children?.length);
    const row: MenuRow = {
      ...item,
      level,
      hasChildren,
    };

    const nextRows = hasChildren
      ? flattenMenuTree(item.children ?? [], level + 1)
      : [];
    return [row, ...nextRows];
  });
}

export function collectDescendantIds(menu?: SystemMenuRecord | null) {
  const ids = new Set<number>();

  const walk = (items?: SystemMenuRecord[]) => {
    items?.forEach((item) => {
      ids.add(item.id);
      walk(item.children);
    });
  };

  walk(menu?.children);
  return ids;
}

export function menuTreeToSelectNodes(
  items: SystemMenuTreeRecord[],
  disabledIds = new Set<number>(),
): TreeSelectNode[] {
  return items.map((item) => ({
    id: item.id,
    label: `${item.menuName} / ${typeLabelMap[item.menuType]}`,
    disabled: disabledIds.has(item.id) || item.status === 0,
    children: item.children?.length
      ? menuTreeToSelectNodes(item.children, disabledIds)
      : undefined,
  }));
}
