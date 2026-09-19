import { z } from "zod";
import type {
  ApiStatus,
  SystemDictDataRecord,
  SystemDictTypeRecord,
} from "@/types";

export type TypeFilterState = {
  dictName: string;
  dictCode: string;
  status: "all" | ApiStatus;
};

export type ItemFilterState = {
  dictLabel: string;
  dictValue: string;
};

export type FormMode = "create" | "edit";

export const DEFAULT_TYPE_FILTERS: TypeFilterState = {
  dictName: "",
  dictCode: "",
  status: "all",
};

export const DEFAULT_ITEM_FILTERS: ItemFilterState = {
  dictLabel: "",
  dictValue: "",
};

const emptyToUndefined = (value: unknown) =>
  value === "" || value === null ? undefined : value;

export const dictTypeFormSchema = z.object({
  dictName: z
    .string()
    .trim()
    .min(1, "字典名称不能为空")
    .max(64, "字典名称不能超过 64 个字符"),
  dictCode: z
    .string()
    .trim()
    .min(1, "字典编码不能为空")
    .max(64, "字典编码不能超过 64 个字符"),
  status: z.coerce.number().pipe(z.union([z.literal(0), z.literal(1)])),
  sortOrder: z.coerce.number().int("排序必须是整数").min(0, "排序不能小于 0"),
  remark: z.preprocess(
    emptyToUndefined,
    z.string().trim().max(200, "备注不能超过 200 个字符").optional(),
  ),
});

export const dictDataFormSchema = z.object({
  dictLabel: z
    .string()
    .trim()
    .min(1, "字典项名称不能为空")
    .max(64, "字典项名称不能超过 64 个字符"),
  dictValue: z
    .string()
    .trim()
    .min(1, "字典项值不能为空")
    .max(128, "字典项值不能超过 128 个字符"),
  sortOrder: z.coerce.number().int("排序必须是整数").min(0, "排序不能小于 0"),
  remark: z.preprocess(
    emptyToUndefined,
    z.string().trim().max(200, "备注不能超过 200 个字符").optional(),
  ),
});

export type DictTypeFormValues = z.infer<typeof dictTypeFormSchema>;
export type DictDataFormValues = z.infer<typeof dictDataFormSchema>;

export function buildTypeQuery(
  filters: TypeFilterState,
  page: number,
  pageSize: number,
) {
  return {
    page,
    pageSize,
    dictName: filters.dictName.trim() || undefined,
    dictCode: filters.dictCode.trim() || undefined,
    status: filters.status === "all" ? undefined : filters.status,
  };
}

export function buildDataQuery(
  filters: ItemFilterState,
  dictTypeId: number | null,
  page: number,
  pageSize: number,
) {
  return {
    page,
    pageSize,
    dictTypeId: dictTypeId ?? undefined,
    dictLabel: filters.dictLabel.trim() || undefined,
    dictValue: filters.dictValue.trim() || undefined,
  };
}

export function toTypeFormValues(
  dictType?: SystemDictTypeRecord,
): DictTypeFormValues {
  return {
    dictName: dictType?.dictName ?? "",
    dictCode: dictType?.dictCode ?? "",
    status: dictType?.status === 0 ? 0 : 1,
    sortOrder: dictType?.sortOrder ?? 0,
    remark: dictType?.remark ?? "",
  };
}

export function toDataFormValues(
  dictData?: SystemDictDataRecord,
): DictDataFormValues {
  return {
    dictLabel: dictData?.dictLabel ?? "",
    dictValue: dictData?.dictValue ?? "",
    sortOrder: dictData?.sortOrder ?? 0,
    remark: dictData?.remark ?? "",
  };
}

export function buildTypePayload(values: DictTypeFormValues) {
  return {
    dictName: values.dictName.trim(),
    dictCode: values.dictCode.trim(),
    status: values.status,
    sortOrder: values.sortOrder,
    remark: values.remark?.trim(),
  };
}

export function buildDataPayload(
  values: DictDataFormValues,
  dictTypeId: number,
) {
  return {
    dictTypeId,
    dictLabel: values.dictLabel.trim(),
    dictValue: values.dictValue.trim(),
    sortOrder: values.sortOrder,
    remark: values.remark?.trim(),
  };
}
