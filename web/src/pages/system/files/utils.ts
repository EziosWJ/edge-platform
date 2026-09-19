const SIZE_UNITS = ["B", "KB", "MB", "GB"] as const;

export function formatFileSize(bytes: number | undefined | null): string {
  if (bytes === undefined || bytes === null || bytes < 0) return "-";
  if (bytes === 0) return "0 B";

  let unitIndex = 0;
  let size = bytes;

  while (size >= 1024 && unitIndex < SIZE_UNITS.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }

  return unitIndex === 0
    ? `${size} ${SIZE_UNITS[unitIndex]}`
    : `${size.toFixed(size >= 100 ? 0 : size >= 10 ? 1 : 2)} ${SIZE_UNITS[unitIndex]}`;
}

export function isPreviewable(mimeType: string | undefined | null): boolean {
  if (!mimeType) return false;
  return mimeType.startsWith("image/") || mimeType === "application/pdf";
}

export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(url);
}
