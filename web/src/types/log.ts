import type { ApiPageRequest } from "@/types/api";

export type LogStatus = "SUCCESS" | "FAIL" | "FAILED" | "ERROR" | string;

export type LoginLogRecord = {
  id: number;
  username: string;
  loginStatus: LogStatus;
  loginIp: string;
  browser: string;
  os: string;
  message: string;
  loginTime: string;
};

export type LoginLogListQuery = ApiPageRequest & {
  username?: string;
  loginStatus?: string;
  loginIp?: string;
};

export type OperLogRecord = {
  id: number;
  moduleName: string;
  operationType: string;
  requestMethod: string;
  requestUrl: string;
  operatorName: string;
  operatorIp: string;
  operationStatus: LogStatus;
  costTime: number;
  operationTime: string;
};

export type OperLogDetail = OperLogRecord & {
  requestParams?: string;
  responseResult?: string;
  errorMessage?: string;
};

export type OperLogListQuery = ApiPageRequest & {
  moduleName?: string;
  operationType?: string;
  operatorName?: string;
  operationStatus?: string;
};
