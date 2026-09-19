import type {
  ApiStatus,
  SystemConfigType,
  SystemConfigValueType,
  SystemMenuType,
  UserGender,
} from "@/types";

export type DictSelectOption<T extends string | number = string> = {
  label: string;
  value: T;
};

export const DICT_CODES = {
  COMMON_STATUS: "COMMON_STATUS",
  CONFIG_TYPE: "CONFIG_TYPE",
  CONFIG_VALUE_TYPE: "CONFIG_VALUE_TYPE",
  FILE_BUSINESS_MODULE: "FILE_BUSINESS_MODULE",
  LOG_STATUS: "LOG_STATUS",
  MENU_TYPE: "MENU_TYPE",
  MENU_VISIBLE: "MENU_VISIBLE",
  OPERATION_TYPE: "OPERATION_TYPE",
  USER_GENDER: "USER_GENDER",
} as const;

export const MENU_TYPE_VALUES = ["DIR", "MENU", "LINK"] as const;
export const API_STATUS_VALUES = [1, 0] as const;
export const USER_GENDER_VALUES = ["UNKNOWN", "MALE", "FEMALE"] as const;
export const CONFIG_TYPE_VALUES = ["SYSTEM", "CUSTOM"] as const;
export const CONFIG_VALUE_TYPE_VALUES = ["TEXT", "NUMBER", "BOOLEAN"] as const;

export const COMMON_STATUS_OPTIONS = [
  { label: "启用", value: 1 },
  { label: "禁用", value: 0 },
] as const satisfies readonly DictSelectOption<ApiStatus>[];

export const MENU_VISIBLE_OPTIONS = [
  { label: "显示", value: 1 },
  { label: "隐藏", value: 0 },
] as const satisfies readonly DictSelectOption<ApiStatus>[];

export const MENU_TYPE_OPTIONS = [
  { label: "目录", value: "DIR" },
  { label: "菜单", value: "MENU" },
  { label: "外链", value: "LINK" },
] as const satisfies readonly DictSelectOption<SystemMenuType>[];

export const USER_GENDER_OPTIONS = [
  { label: "未知", value: "UNKNOWN" },
  { label: "男", value: "MALE" },
  { label: "女", value: "FEMALE" },
] as const satisfies readonly DictSelectOption<UserGender>[];

export const CONFIG_TYPE_OPTIONS = [
  { label: "系统配置", value: "SYSTEM" },
  { label: "自定义配置", value: "CUSTOM" },
] as const satisfies readonly DictSelectOption<SystemConfigType>[];

export const CONFIG_VALUE_TYPE_OPTIONS = [
  { label: "文本", value: "TEXT" },
  { label: "数字", value: "NUMBER" },
  { label: "布尔", value: "BOOLEAN" },
] as const satisfies readonly DictSelectOption<SystemConfigValueType>[];

export const LOG_STATUS_OPTIONS = [
  { label: "成功", value: "SUCCESS" },
  { label: "失败", value: "FAIL" },
] as const satisfies readonly DictSelectOption[];

export const OPERATION_TYPE_OPTIONS = [
  { label: "新增", value: "CREATE" },
  { label: "修改", value: "UPDATE" },
  { label: "删除", value: "DELETE" },
  { label: "查询", value: "QUERY" },
  { label: "导出", value: "EXPORT" },
  { label: "导入", value: "IMPORT" },
] as const satisfies readonly DictSelectOption[];
