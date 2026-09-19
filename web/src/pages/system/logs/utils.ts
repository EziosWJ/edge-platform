import type { OperLogDetail } from "@/types";

type ViteImportMeta = ImportMeta & {
  env?: {
    DEV?: boolean;
  };
};

const operationTypeLabelMap: Record<string, string> = {
  CREATE: "新增",
  UPDATE: "修改",
  DELETE: "删除",
  QUERY: "查询",
  EXPORT: "导出",
  IMPORT: "导入",
  LOGIN: "登录",
  LOGOUT: "退出",
};

export const isDev = Boolean((import.meta as ViteImportMeta).env?.DEV);

export function getStatusMeta(status: string) {
  if (status === "SUCCESS") {
    return { label: "成功", tone: "success" as const };
  }

  if (status === "FAIL" || status === "FAILED" || status === "ERROR") {
    return { label: "失败", tone: "error" as const };
  }

  return { label: status || "-", tone: "neutral" as const };
}

export function getOperationTypeLabel(type: string) {
  return operationTypeLabelMap[type] ?? type ?? "-";
}

function stringifySummary(value: unknown) {
  if (value === null || value === undefined || value === "") return "-";
  if (typeof value === "string") return value;

  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export function getDetailSummary(
  detail: OperLogDetail | null,
  key: "requestParams" | "responseResult",
) {
  if (!detail) return "-";
  const record = detail as Record<string, unknown>;
  const value = record[key];
  return stringifySummary(value);
}
