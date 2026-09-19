import { isApiError } from "@/lib/api-error";

export function getErrorMessage(error: unknown, fallback = "操作失败") {
  if (isApiError(error)) return error.message;
  if (error instanceof Error) return error.message;
  return fallback;
}
