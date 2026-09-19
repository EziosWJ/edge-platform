import type {
  ConfigQuery,
  DictItemRecord,
  DictQuery,
  DictTypeRecord,
  SystemConfigRecord,
} from "@/types";

export const mockDictTypes: DictTypeRecord[] = [
  {
    id: 1,
    name: "用户状态",
    code: "user_status",
    status: "enabled",
    itemCount: 3,
    updatedAt: "2026-05-01 10:20",
  },
  {
    id: 2,
    name: "配置类型",
    code: "config_type",
    status: "enabled",
    itemCount: 4,
    updatedAt: "2026-04-30 16:08",
  },
  {
    id: 3,
    name: "通知渠道",
    code: "notice_channel",
    status: "enabled",
    itemCount: 3,
    updatedAt: "2026-04-28 09:42",
  },
  {
    id: 4,
    name: "旧版业务类型",
    code: "legacy_business_type",
    status: "disabled",
    itemCount: 2,
    updatedAt: "2026-04-20 14:16",
  },
];

export const mockDictItems: DictItemRecord[] = [
  {
    id: 101,
    typeCode: "user_status",
    label: "启用",
    value: "active",
    sort: 1,
    status: "enabled",
    updatedAt: "2026-05-01 10:20",
  },
  {
    id: 102,
    typeCode: "user_status",
    label: "待审核",
    value: "pending",
    sort: 2,
    status: "enabled",
    updatedAt: "2026-05-01 10:20",
  },
  {
    id: 103,
    typeCode: "user_status",
    label: "停用",
    value: "disabled",
    sort: 3,
    status: "enabled",
    updatedAt: "2026-05-01 10:20",
  },
  {
    id: 201,
    typeCode: "config_type",
    label: "字符串",
    value: "string",
    sort: 1,
    status: "enabled",
    updatedAt: "2026-04-30 16:08",
  },
  {
    id: 202,
    typeCode: "config_type",
    label: "数字",
    value: "number",
    sort: 2,
    status: "enabled",
    updatedAt: "2026-04-30 16:08",
  },
  {
    id: 203,
    typeCode: "config_type",
    label: "布尔值",
    value: "boolean",
    sort: 3,
    status: "enabled",
    updatedAt: "2026-04-30 16:08",
  },
  {
    id: 204,
    typeCode: "config_type",
    label: "JSON",
    value: "json",
    sort: 4,
    status: "enabled",
    updatedAt: "2026-04-30 16:08",
  },
  {
    id: 301,
    typeCode: "notice_channel",
    label: "站内信",
    value: "inbox",
    sort: 1,
    status: "enabled",
    updatedAt: "2026-04-28 09:42",
  },
  {
    id: 302,
    typeCode: "notice_channel",
    label: "邮件",
    value: "email",
    sort: 2,
    status: "enabled",
    updatedAt: "2026-04-28 09:42",
  },
  {
    id: 303,
    typeCode: "notice_channel",
    label: "短信",
    value: "sms",
    sort: 3,
    status: "disabled",
    updatedAt: "2026-04-28 09:42",
  },
];

export const mockSystemConfigs: SystemConfigRecord[] = [
  {
    id: 1,
    name: "系统名称",
    key: "system.name",
    value: "智慧综合管理平台",
    type: "string",
    status: "enabled",
    updatedAt: "2026-05-01 11:32",
  },
  {
    id: 2,
    name: "登录失败锁定次数",
    key: "security.loginMaxAttempts",
    value: "5",
    type: "number",
    status: "enabled",
    updatedAt: "2026-04-29 15:18",
  },
  {
    id: 3,
    name: "是否开启维护模式",
    key: "system.maintenance",
    value: "false",
    type: "boolean",
    status: "enabled",
    updatedAt: "2026-04-27 13:06",
  },
  {
    id: 4,
    name: "默认分页配置",
    key: "table.pagination",
    value: "{\"pageSize\":20}",
    type: "json",
    status: "enabled",
    updatedAt: "2026-04-25 17:44",
  },
  {
    id: 5,
    name: "旧版上传地址",
    key: "legacy.uploadUrl",
    value: "/api/legacy/upload",
    type: "string",
    status: "disabled",
    updatedAt: "2026-04-18 09:10",
  },
];

export function filterMockDictTypes(query: DictQuery = {}) {
  const keyword = query.keyword?.trim();
  const status = query.status ?? "all";

  return mockDictTypes.filter((item) => {
    const matchedKeyword =
      !keyword || item.name.includes(keyword) || item.code.includes(keyword);
    const matchedStatus = status === "all" || item.status === status;

    return matchedKeyword && matchedStatus;
  });
}

export function filterMockDictItems(typeCode: string, query: DictQuery = {}) {
  const keyword = query.keyword?.trim();
  const status = query.status ?? "all";

  return mockDictItems.filter((item) => {
    const matchedType = item.typeCode === typeCode;
    const matchedKeyword =
      !keyword || item.label.includes(keyword) || item.value.includes(keyword);
    const matchedStatus = status === "all" || item.status === status;

    return matchedType && matchedKeyword && matchedStatus;
  });
}

export function filterMockSystemConfigs(query: ConfigQuery = {}) {
  const keyword = query.keyword?.trim();
  const type = query.type ?? "all";
  const status = query.status ?? "all";

  return mockSystemConfigs.filter((item) => {
    const matchedKeyword =
      !keyword || item.name.includes(keyword) || item.key.includes(keyword);
    const matchedType = type === "all" || item.type === type;
    const matchedStatus = status === "all" || item.status === status;

    return matchedKeyword && matchedType && matchedStatus;
  });
}
