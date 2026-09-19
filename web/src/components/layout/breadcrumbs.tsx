import { useMemo } from "react";
import { ChevronRight, Home } from "lucide-react";
import { Link, useLocation } from "react-router-dom";
import {
  createUserMenuTitleMap,
  staticRouteTitleMap,
} from "@/config/navigation";
import { useAuthStore } from "@/store/auth-store";

function getRouteTitle(pathname: string, titleMap: Record<string, string>) {
  return (
    titleMap[pathname] ??
    Object.entries(titleMap)
      .sort((a, b) => b[0].length - a[0].length)
      .find(([path]) => pathname.startsWith(`${path}/`))?.[1] ??
    "页面"
  );
}

export function Breadcrumbs() {
  const location = useLocation();
  const menus = useAuthStore((state) => state.menus);
  const dynamicTitleMap = useMemo(() => createUserMenuTitleMap(menus), [menus]);
  const title = getRouteTitle(location.pathname, {
    ...dynamicTitleMap,
    ...staticRouteTitleMap,
  });

  return (
    <nav aria-label="面包屑" className="flex items-center text-sm">
      <Link
        to="/dashboard"
        className="inline-flex items-center gap-1 text-text-tertiary hover:text-primary"
      >
        <Home className="h-4 w-4" aria-hidden />
        首页
      </Link>
      <ChevronRight className="mx-2 h-4 w-4 text-text-tertiary" aria-hidden />
      <span className="font-medium text-text-primary">{title}</span>
    </nav>
  );
}
