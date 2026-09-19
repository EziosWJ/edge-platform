function normalizeDateString(value: string) {
  const trimmed = value.trim();
  if (!trimmed) return "";

  return trimmed.replace("T", " ").replace(/\.\d{1,3}Z?$/i, "").trim();
}

export function formatDateOnly(value?: string | null) {
  if (!value) return "-";

  const normalized = normalizeDateString(value);
  if (!normalized) return "-";

  return normalized.slice(0, 10) || "-";
}

export function formatDateTime(value?: string | null) {
  if (!value) return "-";

  const normalized = normalizeDateString(value);
  if (!normalized) return "-";

  return normalized.slice(0, 19) || "-";
}
