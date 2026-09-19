import { StatusTag } from "@/components/common/status-tag";

type ApiStatusTagValue = 1 | 0 | string | number | null | undefined;

type StatusTone = "success" | "warning" | "error" | "info" | "neutral";

type ApiStatusTagProps = {
  status?: ApiStatusTagValue;
  labels?: Partial<Record<string | number, string>>;
};

const statusTones: Record<string, StatusTone> = {
  "1": "success",
  "0": "neutral",
};

const defaultLabels: Record<string, string> = {
  "1": "启用",
  "0": "禁用",
};

export function ApiStatusTag({ status, labels }: ApiStatusTagProps) {
  const statusKey = status === null || status === undefined ? "" : String(status);
  const label =
    labels?.[statusKey] ?? defaultLabels[statusKey] ?? (statusKey || "-");
  const tone = statusTones[statusKey] ?? "neutral";

  return <StatusTag tone={tone}>{label}</StatusTag>;
}
