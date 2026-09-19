import { z } from "zod";
import type { ApiStatus, DeptRecord } from "@/types";

export type FilterState = {
  deptName: string;
  deptCode: string;
  status: "all" | ApiStatus;
};

export type DeptFormMode = "create" | "edit";

export const DEFAULT_FILTERS: FilterState = {
  deptName: "",
  deptCode: "",
  status: "all",
};

const emptyToUndefined = (value: unknown) =>
  value === "" || value === null ? undefined : value;

export const deptFormSchema = z.object({
  parentId: z.preprocess(
    emptyToUndefined,
    z.coerce
      .number()
      .int("上级部门 ID 必须是整数")
      .min(0, "上级部门 ID 不能小于 0")
      .optional(),
  ),
  deptName: z
    .string()
    .trim()
    .min(1, "部门名称不能为空")
    .max(64, "部门名称不能超过 64 个字符"),
  deptCode: z
    .string()
    .trim()
    .min(1, "部门编码不能为空")
    .max(64, "部门编码不能超过 64 个字符"),
  leader: z.preprocess(
    emptyToUndefined,
    z.string().trim().max(32, "负责人不能超过 32 个字符").optional(),
  ),
  phone: z.preprocess(
    emptyToUndefined,
    z.string().trim().max(20, "联系电话不能超过 20 个字符").optional(),
  ),
  email: z.preprocess(
    emptyToUndefined,
    z.string().trim().email("邮箱格式不正确").optional(),
  ),
  sortOrder: z.coerce.number().int("排序必须是整数").min(0, "排序不能小于 0"),
  status: z.coerce.number().pipe(z.union([z.literal(0), z.literal(1)])),
  remark: z.preprocess(
    emptyToUndefined,
    z.string().trim().max(200, "备注不能超过 200 个字符").optional(),
  ),
});

export type DeptFormValues = z.infer<typeof deptFormSchema>;

export function buildQuery(
  filters: FilterState,
  page: number,
  pageSize: number,
) {
  return {
    page,
    pageSize,
    deptName: filters.deptName.trim() || undefined,
    deptCode: filters.deptCode.trim() || undefined,
    status: filters.status === "all" ? undefined : filters.status,
  };
}

export function buildDeptPayload(values: DeptFormValues) {
  return {
    parentId: values.parentId ?? 0,
    deptName: values.deptName.trim(),
    deptCode: values.deptCode.trim(),
    leader: values.leader?.trim(),
    phone: values.phone?.trim(),
    email: values.email?.trim(),
    sortOrder: values.sortOrder,
    status: values.status,
    remark: values.remark?.trim(),
  };
}

export function toFormValues(dept?: DeptRecord): DeptFormValues {
  return {
    parentId: dept?.parentId ?? undefined,
    deptName: dept?.deptName ?? "",
    deptCode: dept?.deptCode ?? "",
    leader: dept?.leader ?? "",
    phone: dept?.phone ?? "",
    email: dept?.email ?? "",
    sortOrder: dept?.sortOrder ?? 0,
    status: dept?.status === 0 ? 0 : 1,
    remark: dept?.remark ?? "",
  };
}
