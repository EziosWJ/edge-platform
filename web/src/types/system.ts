import type { ApiBuiltinFlag, ApiPageRequest, ApiStatus } from "./api";

export type SystemStatus = "enabled" | "disabled";

export type DictTypeLegacyRecord = {
  id: number;
  name: string;
  code: string;
  status: SystemStatus;
  itemCount: number;
  updatedAt: string;
};

export type DictItemLegacyRecord = {
  id: number;
  typeCode: string;
  label: string;
  value: string;
  sort: number;
  status: SystemStatus;
  updatedAt: string;
};

export type DictTypeRecord = DictTypeLegacyRecord;

export type DictItemRecord = DictItemLegacyRecord;

export type SystemDictTypeRecord = {
  id: number;
  dictName: string;
  dictCode: string;
  status: ApiStatus;
  sortOrder: number;
  isBuiltin: ApiBuiltinFlag;
  remark?: string | null;
  createTime?: string | null;
};

export type SystemDictDataRecord = {
  id: number;
  dictTypeId: number;
  dictLabel: string;
  dictValue: string;
  sortOrder: number;
  remark?: string | null;
  createTime?: string | null;
};

export type DictQuery = {
  keyword?: string;
  status?: SystemStatus | "all";
};

export type DictTypeListQuery = Partial<ApiPageRequest> & {
  dictName?: string;
  dictCode?: string;
  status?: ApiStatus;
};

export type DictTypeCreateRequest = {
  dictName: string;
  dictCode: string;
  status: ApiStatus;
  sortOrder: number;
  remark?: string;
};

export type DictTypeUpdateRequest = DictTypeCreateRequest;

export type DictTypeStatusRequest = {
  status: ApiStatus;
};

export type DictTypeBatchDeleteRequest = {
  ids: number[];
};

export type DictDataListQuery = Partial<ApiPageRequest> & {
  dictTypeId?: number;
  dictCode?: string;
  dictLabel?: string;
  dictValue?: string;
};

export type DictDataCreateRequest = {
  dictTypeId: number;
  dictLabel: string;
  dictValue: string;
  sortOrder: number;
  remark?: string;
};

export type DictDataUpdateRequest = DictDataCreateRequest;

export type DictDataBatchDeleteRequest = {
  ids: number[];
};

export type DictOption = {
  label: string;
  value: string;
  sortOrder: number;
};

export type ConfigType = "string" | "number" | "boolean" | "json";

export type SystemConfigType = "SYSTEM" | "CUSTOM";

export type SystemConfigValueType = "TEXT" | "NUMBER" | "BOOLEAN";

export type SystemConfigLegacyRecord = {
  id: number;
  name: string;
  key: string;
  value: string;
  type: ConfigType;
  status: SystemStatus;
  updatedAt: string;
};

export type ConfigQuery = {
  keyword?: string;
  type?: ConfigType | "all";
  status?: SystemStatus | "all";
};

export type SystemConfigRecord = {
  id: number;
  configName?: string;
  configKey?: string;
  configValue?: string | null;
  configType?: SystemConfigType;
  valueType?: SystemConfigValueType;
  status: ApiStatus | SystemStatus;
  isBuiltin?: ApiBuiltinFlag;
  remark?: string | null;
  createTime?: string | null;
  updateTime?: string | null;

  /**
   * 兼容旧 mock 数据类型。系统配置页面和真实接口只使用上方后端字段。
   */
  name: string;
  key: string;
  value: string;
  type: ConfigType;
  updatedAt: string;
};

export type SystemConfigValueRecord = {
  configKey: string;
  configValue?: string | null;
  valueType: SystemConfigValueType;
  configName: string;
};

export type SystemConfigListQuery = Partial<ApiPageRequest> & {
  configName?: string;
  configKey?: string;
  configType?: SystemConfigType;
  status?: ApiStatus;
};

export type SystemConfigCreateRequest = {
  configName: string;
  configKey: string;
  configValue?: string;
  configType: SystemConfigType;
  valueType: SystemConfigValueType;
  status: ApiStatus;
  remark?: string;
};

export type SystemConfigUpdateRequest = SystemConfigCreateRequest;

export type SystemConfigStatusRequest = {
  status: ApiStatus;
};

export type SystemConfigBatchDeleteRequest = {
  ids: number[];
};
