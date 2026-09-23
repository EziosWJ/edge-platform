import type {
  MenuQuery,
  MenuRecord,
  PermissionQuery,
  PermissionRecord,
  RolePermissionDetail,
  RoleQuery,
  RoleRecord,
} from "@/types";

export const mockRoles: RoleRecord[] = [
  {
    id: 1,
    name: "超级管理员",
    code: "super_admin",
    status: "enabled",
    dataScope: "all",
    userCount: 1,
    sort: 1,
    updatedAt: "2026-05-02 09:28",
  },
  {
    id: 2,
    name: "运营管理员",
    code: "ops_admin",
    status: "enabled",
    dataScope: "department_and_sub",
    userCount: 4,
    sort: 2,
    updatedAt: "2026-05-01 15:12",
  },
  {
    id: 3,
    name: "内容运营",
    code: "content_ops",
    status: "enabled",
    dataScope: "department",
    userCount: 8,
    sort: 3,
    updatedAt: "2026-04-30 17:36",
  },
  {
    id: 4,
    name: "审计人员",
    code: "auditor",
    status: "disabled",
    dataScope: "custom",
    userCount: 2,
    sort: 4,
    updatedAt: "2026-04-28 11:20",
  },
];

export const mockMenus: MenuRecord[] = [
  {
    id: 1,
    parentId: null,
    name: "系统管理",
    type: "directory",
    routePath: "/system",
    componentPath: "",
    permission: "",
    icon: "Settings",
    sort: 1,
    status: "enabled",
    updatedAt: "2026-05-02 11:16",
    children: [
      {
        id: 11,
        parentId: 1,
        name: "字典管理",
        type: "menu",
        routePath: "/system/dict",
        componentPath: "@/pages/system/dict",
        permission: "system:dict:list",
        icon: "ListTree",
        sort: 1,
        status: "enabled",
        updatedAt: "2026-05-02 11:16",
      },
      {
        id: 12,
        parentId: 1,
        name: "配置管理",
        type: "menu",
        routePath: "/system/config",
        componentPath: "@/pages/system/config",
        permission: "system:config:list",
        icon: "SlidersHorizontal",
        sort: 2,
        status: "enabled",
        updatedAt: "2026-05-02 11:16",
        children: [
          {
            id: 121,
            parentId: 12,
            name: "配置新增",
            type: "button",
            routePath: "",
            componentPath: "",
            permission: "system:config:add",
            icon: "Plus",
            sort: 1,
            status: "enabled",
            updatedAt: "2026-05-02 11:16",
          },
        ],
      },
    ],
  },
  {
    id: 2,
    parentId: null,
    name: "RBAC 管理",
    type: "directory",
    routePath: "/rbac",
    componentPath: "",
    permission: "",
    icon: "ShieldHalf",
    sort: 2,
    status: "enabled",
    updatedAt: "2026-05-02 11:16",
    children: [
      {
        id: 21,
        parentId: 2,
        name: "角色管理",
        type: "menu",
        routePath: "/system/role",
        componentPath: "@/pages/system/role",
        permission: "system:role:list",
        icon: "ShieldHalf",
        sort: 1,
        status: "enabled",
        updatedAt: "2026-05-02 11:16",
      },
      {
        id: 22,
        parentId: 2,
        name: "菜单管理",
        type: "menu",
        routePath: "/system/menu",
        componentPath: "@/pages/system/menu",
        permission: "system:menu:list",
        icon: "GitBranch",
        sort: 2,
        status: "enabled",
        updatedAt: "2026-05-02 11:16",
      },
    ],
  },
];

export const mockPermissions: PermissionRecord[] = [
  {
    id: 1,
    name: "角色列表查看",
    code: "system:role:list",
    menuName: "角色管理",
    type: "page",
    status: "enabled",
    sort: 1,
    description: "查看角色列表页面",
    updatedAt: "2026-05-02 11:40",
  },
  {
    id: 2,
    name: "角色新增",
    code: "system:role:add",
    menuName: "角色管理",
    type: "button",
    status: "enabled",
    sort: 2,
    description: "新增角色按钮权限",
    updatedAt: "2026-05-02 11:40",
  },
  {
    id: 3,
    name: "菜单列表查看",
    code: "system:menu:list",
    menuName: "菜单管理",
    type: "page",
    status: "enabled",
    sort: 1,
    description: "查看菜单列表页面",
    updatedAt: "2026-05-02 11:40",
  },
  {
    id: 4,
    name: "权限点删除",
    code: "system:permission:delete",
    menuName: "权限点管理",
    type: "api",
    status: "disabled",
    sort: 4,
    description: "删除权限点接口",
    updatedAt: "2026-05-02 11:40",
  },
  {
    id: 5,
    name: "数据导出",
    code: "system:permission:export",
    menuName: "权限点管理",
    type: "data",
    status: "enabled",
    sort: 5,
    description: "数据维度权限点",
    updatedAt: "2026-05-02 11:40",
  },
  {
    id: 6,
    name: "Command 列表查看",
    code: "command:list",
    menuName: "Command 管理",
    type: "page",
    status: "enabled",
    sort: 6,
    description: "查看 Cloud Command 审计列表",
    updatedAt: "2026-05-02 11:40",
  },
  {
    id: 7,
    name: "Command 详情查看",
    code: "command:detail",
    menuName: "Command 管理",
    type: "api",
    status: "enabled",
    sort: 7,
    description: "查看 Cloud Command 详情",
    updatedAt: "2026-05-02 11:40",
  },
  {
    id: 8,
    name: "Command 执行",
    code: "command:execute",
    menuName: "Command 管理",
    type: "button",
    status: "enabled",
    sort: 8,
    description: "从 Device 发起 Command",
    updatedAt: "2026-05-02 11:40",
  },
  {
    id: 9,
    name: "HMI 页面列表",
    code: "hmi:list",
    menuName: "HMI 页面",
    type: "page",
    status: "enabled",
    sort: 9,
    description: "查看 HMI 页面",
    updatedAt: "2026-09-23 10:00",
  },
  {
    id: 10,
    name: "HMI 页面详情",
    code: "hmi:detail",
    menuName: "HMI 页面",
    type: "api",
    status: "enabled",
    sort: 10,
    description: "查看 HMI Draft 详情",
    updatedAt: "2026-09-23 10:00",
  },
  {
    id: 11,
    name: "HMI 页面编辑",
    code: "hmi:edit",
    menuName: "HMI 页面",
    type: "button",
    status: "enabled",
    sort: 11,
    description: "创建和编辑 HMI Draft",
    updatedAt: "2026-09-23 10:00",
  },
  {
    id: 12,
    name: "HMI 页面发布",
    code: "hmi:publish",
    menuName: "HMI 页面",
    type: "button",
    status: "enabled",
    sort: 12,
    description: "发布 HMI 页面版本",
    updatedAt: "2026-09-23 10:00",
  },
  {
    id: 13,
    name: "HMI 页面运行",
    code: "hmi:run",
    menuName: "HMI 页面",
    type: "page",
    status: "enabled",
    sort: 13,
    description: "查看已发布 HMI 页面",
    updatedAt: "2026-09-23 10:00",
  },
];

export const mockCurrentUserPermissionCodes: string[] = [
  "command:list",
  "command:detail",
  "command:execute",
  "hmi:list",
  "hmi:detail",
  "hmi:edit",
  "hmi:publish",
  "hmi:run",
  "system:role:list",
  "system:role:add",
  "system:role:edit",
  "system:menu:list",
  "system:menu:add",
  "system:menu:edit",
  "system:dict:list",
  "system:config:list",
  "system:permission:list",
  "system:permission:add",
] as const;

export const mockRolePermissionDetails: Record<number, RolePermissionDetail> = {
  1: {
    roleId: 1,
    roleName: "超级管理员",
    roleCode: "super_admin",
    status: "enabled",
    dataScope: "all",
    menuIds: [1, 11, 12, 121, 2, 21, 22],
    permissionIds: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13],
  },
  2: {
    roleId: 2,
    roleName: "运营管理员",
    roleCode: "ops_admin",
    status: "enabled",
    dataScope: "department_and_sub",
    menuIds: [1, 11, 12, 21],
    permissionIds: [1, 2, 3],
  },
  3: {
    roleId: 3,
    roleName: "内容运营",
    roleCode: "content_ops",
    status: "enabled",
    dataScope: "department",
    menuIds: [11, 12, 121, 21],
    permissionIds: [1, 2, 3],
  },
  4: {
    roleId: 4,
    roleName: "审计人员",
    roleCode: "auditor",
    status: "disabled",
    dataScope: "custom",
    menuIds: [11, 21],
    permissionIds: [1, 4],
  },
};

function flattenMenus(items: MenuRecord[]): MenuRecord[] {
  const result: MenuRecord[] = [];

  for (const item of items) {
    const { children, ...rest } = item;
    result.push(rest);
    if (children?.length) {
      result.push(...flattenMenus(children));
    }
  }

  return result;
}

function countMenus(items: MenuRecord[]): number {
  return items.reduce((count, item) => {
    return count + 1 + (item.children?.length ? countMenus(item.children) : 0);
  }, 0);
}

export function filterMockRoles(query: RoleQuery = {}) {
  const keyword = query.keyword?.trim();
  const status = query.status ?? "all";
  const dataScope = query.dataScope ?? "all";

  return mockRoles.filter((item) => {
    const matchedKeyword =
      !keyword || item.name.includes(keyword) || item.code.includes(keyword);
    const matchedStatus = status === "all" || item.status === status;
    const matchedScope = dataScope === "all" || item.dataScope === dataScope;

    return matchedKeyword && matchedStatus && matchedScope;
  });
}

export function filterMockMenus(query: MenuQuery = {}) {
  const keyword = query.keyword?.trim();
  const type = query.type ?? "all";
  const status = query.status ?? "all";

  return flattenMenus(mockMenus).filter((item) => {
    const matchedKeyword =
      !keyword ||
      item.name.includes(keyword) ||
      item.routePath.includes(keyword) ||
      item.permission.includes(keyword);
    const matchedType = type === "all" || item.type === type;
    const matchedStatus = status === "all" || item.status === status;

    return matchedKeyword && matchedType && matchedStatus;
  });
}

export function getMockMenuTotal() {
  return countMenus(mockMenus);
}

export function filterMockPermissions(query: PermissionQuery = {}) {
  const keyword = query.keyword?.trim();
  const type = query.type ?? "all";
  const status = query.status ?? "all";

  return mockPermissions.filter((item) => {
    const matchedKeyword =
      !keyword ||
      item.name.includes(keyword) ||
      item.code.includes(keyword) ||
      item.menuName.includes(keyword) ||
      item.description.includes(keyword);
    const matchedType = type === "all" || item.type === type;
    const matchedStatus = status === "all" || item.status === status;

    return matchedKeyword && matchedType && matchedStatus;
  });
}

export function getMockPermissionTotal() {
  return mockPermissions.length;
}

export function getMockRolePermissionDetail(
  roleId: number,
): RolePermissionDetail | null {
  return mockRolePermissionDetails[roleId] ?? null;
}
