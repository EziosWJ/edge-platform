import { ApiError, getApiErrorType } from "@/lib/api-error";
import type {
  ApiFieldErrors,
  ApiQueryParams,
  ApiQueryValue,
  ApiResponse,
} from "@/types/api";

type AuthTokenGetter = () => string | null | undefined;
type UnauthorizedHandler = (error: ApiError) => void | Promise<void>;
type HttpMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
type ResponseType = "json" | "blob";
type ViteImportMeta = ImportMeta & {
  env?: {
    VITE_API_BASE_URL?: string;
  };
};

type HttpRequestOptions = {
  query?: ApiQueryParams;
  body?: unknown;
  headers?: HeadersInit;
  signal?: AbortSignal;
  credentials?: RequestCredentials;
};

type HttpRequestOptionsWithResponse = HttpRequestOptions & {
  responseType?: ResponseType;
};

let authTokenGetter: AuthTokenGetter | null = null;
let unauthorizedHandler: UnauthorizedHandler | null = null;
let isRedirecting = false;

export function setAuthTokenGetter(getter: AuthTokenGetter | null) {
  authTokenGetter = getter;
}

export function setUnauthorizedHandler(handler: UnauthorizedHandler | null) {
  unauthorizedHandler = handler;
}

async function notifyUnauthorized(error: ApiError) {
  if (!unauthorizedHandler) return;
  if (isRedirecting) return;
  isRedirecting = true;
  await unauthorizedHandler(error);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isFieldErrors(value: unknown): value is ApiFieldErrors {
  if (!isRecord(value)) return false;
  return Object.values(value).every((item) => typeof item === "string");
}

function appendQueryValue(
  searchParams: URLSearchParams,
  key: string,
  value: ApiQueryValue,
) {
  if (value === null || value === undefined || value === "") return;

  if (Array.isArray(value)) {
    value.forEach((item) => searchParams.append(key, String(item)));
    return;
  }

  searchParams.set(key, String(value));
}

function joinUrl(baseUrl: string, path: string) {
  if (/^https?:\/\//i.test(path)) return path;

  const normalizedBaseUrl = baseUrl.replace(/\/+$/, "");
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;

  return `${normalizedBaseUrl}${normalizedPath}`;
}

export function buildApiUrl(path: string) {
  const baseUrl = (import.meta as ViteImportMeta).env?.VITE_API_BASE_URL ?? "";
  return joinUrl(baseUrl, path);
}

function buildUrl(path: string, query?: ApiQueryParams) {
  const url = buildApiUrl(path);

  if (!query) return url;

  const [pathname, existingQuery = ""] = url.split("?");
  const searchParams = new URLSearchParams(existingQuery);

  Object.entries(query).forEach(([key, value]) => {
    appendQueryValue(searchParams, key, value);
  });

  const queryString = searchParams.toString();
  return queryString ? `${pathname}?${queryString}` : pathname;
}

function buildHeaders(body: unknown, headers?: HeadersInit) {
  const requestHeaders = new Headers(headers);
  const token = authTokenGetter?.();

  if (token) {
    requestHeaders.set("Authorization", token);
  }

  if (body !== undefined && !(body instanceof FormData)) {
    requestHeaders.set("Content-Type", "application/json");
  }

  return requestHeaders;
}

function buildBody(body: unknown) {
  if (body === undefined) return undefined;
  if (body instanceof FormData) return body;
  if (body instanceof Blob) return body;
  if (typeof body === "string") return body;
  return JSON.stringify(body);
}

async function readJson<T>(response: Response): Promise<ApiResponse<T> | null> {
  const text = await response.text();
  if (!text) return null;
  return JSON.parse(text) as ApiResponse<T>;
}

async function parseHttpError(response: Response) {
  try {
    const payload = await readJson<unknown>(response);
    const code = payload?.code ?? response.status;
    const message = payload?.message || response.statusText || "请求失败";
    const error = new ApiError({
      code,
      message,
      status: response.status,
      type: getApiErrorType(code),
      data: payload?.data,
      fieldErrors: isFieldErrors(payload?.data) ? payload.data : undefined,
    });

    if (error.type === "unauthorized") {
      await notifyUnauthorized(error);
    }

    return error;
  } catch {
    const error = new ApiError({
      code: response.status,
      message: response.statusText || "请求失败",
      status: response.status,
      type: getApiErrorType(response.status),
    });

    if (error.type === "unauthorized") {
      await notifyUnauthorized(error);
    }

    return error;
  }
}

async function request<T>(
  method: HttpMethod,
  path: string,
  options: HttpRequestOptionsWithResponse = {},
): Promise<T> {
  const { query, body, headers, signal, credentials, responseType = "json" } =
    options;

  let response: Response;

  try {
    response = await fetch(buildUrl(path, query), {
      method,
      headers: buildHeaders(body, headers),
      body: buildBody(body),
      signal,
      credentials,
    });
  } catch (error) {
    throw new ApiError({
      code: 0,
      message: error instanceof Error ? error.message : "网络请求失败",
      type: "network",
      data: error,
    });
  }

  if (!response.ok) {
    throw await parseHttpError(response);
  }

  if (responseType === "blob") {
    const contentType = response.headers.get("Content-Type") ?? "";
    if (contentType.includes("application/json")) {
      const payload = await readJson<unknown>(response.clone());
      if (payload && payload.code !== 200) {
        const error = new ApiError({
          code: payload.code,
          message: payload.message || "请求失败",
          status: response.status,
          type: getApiErrorType(payload.code),
          data: payload.data,
          fieldErrors: isFieldErrors(payload.data) ? payload.data : undefined,
        });

        if (error.type === "unauthorized") {
          await notifyUnauthorized(error);
        }

        throw error;
      }

      if (payload && payload.code === 200) {
        throw new ApiError({
          code: payload.code,
          message: "服务端返回了 JSON 而非文件流",
          status: response.status,
          type: getApiErrorType(payload.code),
          data: payload.data,
        });
      }
    }

    return response.blob() as Promise<T>;
  }

  const payload = await readJson<T>(response);

  if (!payload) {
    return undefined as T;
  }

  if (payload.code !== 200) {
    const error = new ApiError({
      code: payload.code,
      message: payload.message || "请求失败",
      status: response.status,
      type: getApiErrorType(payload.code),
      data: payload.data,
      fieldErrors: isFieldErrors(payload.data) ? payload.data : undefined,
    });

    if (error.type === "unauthorized") {
      await notifyUnauthorized(error);
    }

    throw error;
  }

  return payload.data;
}

export const http = {
  get<T>(path: string, options?: HttpRequestOptions) {
    return request<T>("GET", path, options);
  },
  post<T>(path: string, body?: unknown, options?: HttpRequestOptions) {
    return request<T>("POST", path, { ...options, body });
  },
  put<T>(path: string, body?: unknown, options?: HttpRequestOptions) {
    return request<T>("PUT", path, { ...options, body });
  },
  patch<T>(path: string, body?: unknown, options?: HttpRequestOptions) {
    return request<T>("PATCH", path, { ...options, body });
  },
  delete<T>(path: string, body?: unknown, options?: HttpRequestOptions) {
    return request<T>("DELETE", path, { ...options, body });
  },
  blob(path: string, options?: HttpRequestOptions) {
    return request<Blob>("GET", path, { ...options, responseType: "blob" });
  },
  request,
};
