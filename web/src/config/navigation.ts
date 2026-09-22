import { Bell, ClipboardList, Cpu, Gauge, LayoutDashboard, Server, type LucideIcon } from "lucide-react";
import { getMenuIcon } from "@/lib/menu-icons";
import type { CurrentUserMenu } from "@/types";

export type NavItem = {
  label: string;
  path: string;
  icon: LucideIcon;
  externalUrl?: string;
  activePaths?: string[];
  children?: NavItem[];
};

export const defaultNavItems: NavItem[] = [
  { label: "工作台", path: "/dashboard", icon: LayoutDashboard },
  { label: "Edge 管理", path: "/edge", icon: Server },
  { label: "Device 管理", path: "/device", icon: Cpu },
  { label: "DataPoint 管理", path: "/datapoint", icon: Gauge },
  { label: "Command 管理", path: "/command", icon: ClipboardList },
  { label: "我的通知", path: "/notifications", icon: Bell },
];

export const notificationManageNavItem: NavItem = {
  label: "通知管理",
  path: "/system/notification",
  icon: Bell,
};

function collectNavPaths(items: NavItem[]) {
  const paths = new Set<string>();
  const walk = (navItems: NavItem[]) => {
    navItems.forEach((item) => {
      paths.add(item.path);
      if (item.externalUrl) paths.add(item.externalUrl);
      if (item.children?.length) walk(item.children);
    });
  };
  walk(items);
  return paths;
}

function filterDuplicateNavItems(items: NavItem[], existingPaths: Set<string>): NavItem[] {
  const nextItems: NavItem[] = [];
  items.forEach((item) => {
    const key = item.externalUrl ?? item.path;
    if (existingPaths.has(key)) return;
    existingPaths.add(key);
    const children = item.children?.length
      ? filterDuplicateNavItems(item.children, existingPaths)
      : undefined;
    if (item.children?.length && !children?.length) return;
    nextItems.push({ ...item, children: children?.length ? children : undefined });
  });
  return nextItems;
}

function toNavItem(menu: CurrentUserMenu): NavItem | null {
  if (menu.visible !== 1) return null;
  const children = [...(menu.children ?? [])]
    .sort((a, b) => a.sortOrder - b.sortOrder)
    .map((child) => toNavItem(child))
    .filter((item): item is NavItem => Boolean(item));

  if (menu.menuType === "DIR" && children.length === 0) return null;
  if (menu.menuType === "LINK" && !menu.externalUrl) return null;

  const externalUrl = menu.menuType === "LINK" ? menu.externalUrl : "";
  const path = externalUrl || menu.path;
  if (!path) return null;

  return {
    label: menu.menuName,
    path,
    icon: getMenuIcon(menu.icon),
    externalUrl: externalUrl || undefined,
    children: children.length > 0 ? children : undefined,
  };
}

export function convertUserMenusToNavItems(menus: CurrentUserMenu[]): NavItem[] {
  return [...menus]
    .sort((a, b) => a.sortOrder - b.sortOrder)
    .map((menu) => toNavItem(menu))
    .filter((item): item is NavItem => Boolean(item));
}

export function mergeNavItems(baseItems: NavItem[], userItems: NavItem[]): NavItem[] {
  return [...baseItems, ...filterDuplicateNavItems(userItems, collectNavPaths(baseItems))];
}

export function createUserMenuTitleMap(menus: CurrentUserMenu[]): Record<string, string> {
  const titleMap: Record<string, string> = {};
  const walk = (items: CurrentUserMenu[]) => {
    items.forEach((item) => {
      if (item.visible !== 1) return;
      const path = item.menuType === "LINK" ? item.externalUrl : item.path;
      if (path) titleMap[path] = item.menuName;
      if (item.children?.length) walk(item.children);
    });
  };
  walk(menus);
  return titleMap;
}

export const staticRouteTitleMap: Record<string, string> = {
  "/dashboard": "工作台",
  "/edge": "Edge 管理",
  "/device": "Device 管理",
  "/datapoint": "DataPoint 管理",
  "/command": "Command 管理",
  "/notifications": "我的通知",
  "/settings": "系统设置",
  "/system": "系统管理",
  "/system/user": "用户管理",
  "/system/dept": "部门管理",
  "/system/dict": "字典管理",
  "/system/config": "配置管理",
  "/system/role": "角色管理",
  "/system/menu": "菜单管理",
  "/system/login-log": "登录日志",
  "/system/oper-log": "操作日志",
  "/system/file": "文件管理",
  "/system/notification": "通知管理",
  "/account/profile": "个人中心",
  "/account/change-password": "修改密码",
};

export const navItems = defaultNavItems;
export const routeTitleMap = staticRouteTitleMap;
