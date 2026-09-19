import { http } from "@/lib/http";
import type { ApiPageResult } from "@/types/api";
import type {
  LoginLogListQuery,
  LoginLogRecord,
  OperLogDetail,
  OperLogListQuery,
  OperLogRecord,
} from "@/types/log";

const LOGIN_LOG_BASE_PATH = "/api/system/login-log";
const OPER_LOG_BASE_PATH = "/api/system/oper-log";

export function getLoginLogPage(query: LoginLogListQuery) {
  return http.get<ApiPageResult<LoginLogRecord>>(`${LOGIN_LOG_BASE_PATH}/page`, {
    query,
  });
}

export function getLoginLogDetail(id: number) {
  return http.get<LoginLogRecord>(`${LOGIN_LOG_BASE_PATH}/${id}`);
}

export function clearLoginLogs() {
  return http.delete<void>(`${LOGIN_LOG_BASE_PATH}/clear`);
}

export function getOperLogPage(query: OperLogListQuery) {
  return http.get<ApiPageResult<OperLogRecord>>(`${OPER_LOG_BASE_PATH}/page`, {
    query,
  });
}

export function getOperLogDetail(id: number) {
  return http.get<OperLogDetail>(`${OPER_LOG_BASE_PATH}/${id}`);
}

export function clearOperLogs() {
  return http.delete<void>(`${OPER_LOG_BASE_PATH}/clear`);
}
