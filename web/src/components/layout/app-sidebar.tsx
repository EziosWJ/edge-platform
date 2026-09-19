import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import { Boxes, ChevronDown } from "lucide-react";
import { toast } from "@/components/common/toast-store";
import {
  convertUserMenusToNavItems,
  defaultNavItems,
  mergeNavItems,
  notificationManageNavItem,
  type NavItem,
} from "@/config/navigation";
import { isApiError } from "@/lib/api-error";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth-store";

type AppSidebarProps = {
  collapsed: boolean;
};

function isPathActive(pathname: string, path: string) {
  if (/^https?:\/\//i.test(path)) return false;
  return pathname === path || pathname.startsWith(`${path}/`);
}

function hasActivePath(pathname: string, item: NavItem): boolean {
  return (
    isPathActive(pathname, item.path) ||
    Boolean(item.activePaths?.some((path) => isPathActive(pathname, path))) ||
    Boolean(item.children?.some((child) => hasActivePath(pathname, child)))
  );
}

function collectActiveGroupPaths(pathname: string, items: NavItem[]) {
  const paths: string[] = [];

  const walk = (navItems: NavItem[]) => {
    navItems.forEach((item) => {
      if (
        item.children?.length &&
        item.children.some((child) => hasActivePath(pathname, child))
      ) {
        paths.push(item.path);
        walk(item.children);
      }
    });
  };

  walk(items);
  return paths;
}

export function AppSidebar({ collapsed }: AppSidebarProps) {
  const location = useLocation();
  const [expandedPaths, setExpandedPaths] = useState<string[]>([]);
  const hasRequestedMenusRef = useRef(false);
  const menus = useAuthStore((state) => state.menus);
  const user = useAuthStore((state) => state.user);
  const isLoadingMenus = useAuthStore((state) => state.isLoadingMenus);
  const fetchCurrentUserMenus = useAuthStore(
    (state) => state.fetchCurrentUserMenus,
  );

  const sidebarNavItems = useMemo(
    () => {
      const base = user?.roles?.some((role) => role.roleCode === "ADMIN")
        ? [...defaultNavItems, notificationManageNavItem]
        : defaultNavItems;
      return mergeNavItems(base, convertUserMenusToNavItems(menus));
    },
    [menus, user],
  );

  const activeGroupPaths = useMemo(
    () => collectActiveGroupPaths(location.pathname, sidebarNavItems),
    [location.pathname, sidebarNavItems],
  );

  useEffect(() => {
    if (activeGroupPaths.length === 0) return;

    setExpandedPaths((current) => {
      const next = new Set(current);
      activeGroupPaths.forEach((path) => next.add(path));
      return Array.from(next);
    });
  }, [activeGroupPaths]);

  useEffect(() => {
    if (hasRequestedMenusRef.current || isLoadingMenus || menus.length > 0) {
      return;
    }

    hasRequestedMenusRef.current = true;
    void fetchCurrentUserMenus().catch((error) => {
      if (isApiError(error) && error.type === "unauthorized") return;

      toast.warning({
        title: "业务菜单加载失败",
        description: "已保留默认导航，请稍后刷新重试。",
      });
    });
  }, [fetchCurrentUserMenus, isLoadingMenus, menus.length]);

  const toggleGroup = (path: string) => {
    setExpandedPaths((current) =>
      current.includes(path)
        ? current.filter((item) => item !== path)
        : [...current, path],
    );
  };

  const renderNavItem = (item: NavItem, depth = 0) => {
    const Icon = item.icon;
    const hasChildren = Boolean(item.children?.length);
    const isExpanded = expandedPaths.includes(item.path);
    // 仅叶子菜单项显示 active 样式，组菜单只展开不标蓝
    const active = hasChildren
      ? location.pathname === item.path
      : hasActivePath(location.pathname, item);
    const itemHeightClass = depth === 0 ? "h-10" : "h-9";
    const itemGapClass = depth === 0 ? "gap-3" : "gap-2";
    const itemTextClass = depth === 0 ? "font-medium" : "";
    const itemClassName = cn(
      "flex w-full items-center rounded-lg px-3 text-sm transition-colors",
      itemHeightClass,
      itemGapClass,
      itemTextClass,
      active
        ? "bg-blue-50 text-primary"
        : "text-text-secondary hover:bg-slate-50 hover:text-text-primary",
      collapsed && depth === 0 && "justify-center px-0",
    );

    return (
      <div key={item.path}>
        {hasChildren ? (
          <button
            type="button"
            title={collapsed ? item.label : undefined}
            aria-expanded={!collapsed && isExpanded}
            className={itemClassName}
            onClick={() => toggleGroup(item.path)}
          >
            <Icon className="h-4 w-4 shrink-0" aria-hidden />
            {!collapsed && <span className="truncate">{item.label}</span>}
            {!collapsed && (
              <ChevronDown
                className={cn(
                  "ml-auto h-4 w-4 shrink-0 transition-transform",
                  isExpanded && "rotate-180",
                )}
                aria-hidden
              />
            )}
          </button>
        ) : item.externalUrl ? (
          <a
            href={item.externalUrl}
            target="_blank"
            rel="noreferrer"
            title={collapsed ? item.label : undefined}
            className={itemClassName}
          >
            <Icon className="h-4 w-4 shrink-0" aria-hidden />
            {!collapsed && <span className="truncate">{item.label}</span>}
          </a>
        ) : (
          <Link
            to={item.path}
            title={collapsed ? item.label : undefined}
            className={itemClassName}
          >
            <Icon className="h-4 w-4 shrink-0" aria-hidden />
            {!collapsed && <span className="truncate">{item.label}</span>}
          </Link>
        )}

        {hasChildren && !collapsed && isExpanded && (
          <div className="mt-1 space-y-1 pl-6">
            {item.children?.map((child) => renderNavItem(child, depth + 1))}
          </div>
        )}
      </div>
    );
  };

  return (
    <aside
      className={cn(
        "fixed inset-y-0 left-0 z-30 hidden border-r border-border bg-surface transition-[width] md:block",
        collapsed ? "w-16" : "w-60",
      )}
    >
      <div className="flex h-16 items-center gap-3 border-b border-border px-4">
        <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary text-white">
          <Boxes className="h-5 w-5" aria-hidden />
        </span>
        {!collapsed && (
          <div className="min-w-0">
            <div className="truncate text-base font-semibold text-text-primary">
              Edge Platform
            </div>
            <div className="truncate text-xs text-text-tertiary">工业 IoT / HMI 云平台</div>
          </div>
        )}
      </div>

      <nav className="space-y-1 px-3 py-4" aria-label="主导航">
        {sidebarNavItems.map((item) => renderNavItem(item))}
      </nav>
    </aside>
  );
}
