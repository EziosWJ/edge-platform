import type { ApiFieldErrors } from "@/types/api";

export type ApiErrorType =
  | "network"
  | "business"
  | "unauthorized"
  | "forbidden"
  | "notFound"
  | "server";

export type ApiErrorOptions = {
  code: number;
  message: string;
  type?: ApiErrorType;
  status?: number;
  data?: unknown;
  fieldErrors?: ApiFieldErrors;
};

export class ApiError extends Error {
  code: number;
  type: ApiErrorType;
  status?: number;
  data?: unknown;
  fieldErrors?: ApiFieldErrors;

  constructor(options: ApiErrorOptions) {
    super(options.message);
    this.name = "ApiError";
    this.code = options.code;
    this.type = options.type ?? getApiErrorType(options.code);
    this.status = options.status;
    this.data = options.data;
    this.fieldErrors = options.fieldErrors;
  }
}

export function getApiErrorType(code: number): ApiErrorType {
  if (code === 401) return "unauthorized";
  if (code === 403) return "forbidden";
  if (code === 404) return "notFound";
  if (code >= 500) return "server";
  return "business";
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError;
}

export function getErrorMessage(error: unknown, fallback: string): string {
  if (isApiError(error)) return error.message;
  if (error instanceof Error) return error.message;
  return fallback;
}
