import { useState } from "react";
import { Outlet } from "react-router-dom";
import { AppHeader } from "@/components/layout/app-header";
import { AppSidebar } from "@/components/layout/app-sidebar";
import { RouteLoadingIndicator } from "@/components/layout/route-loading-indicator";
import { useRouteNavigationLoading } from "@/components/layout/use-route-navigation-loading";
import { cn } from "@/lib/utils";

export function AppShell() {
  const [collapsed, setCollapsed] = useState(false);
  const { navigationIntent, handleNavigationClickCapture } = useRouteNavigationLoading();

  return (
    <div className="min-h-screen bg-background" onClickCapture={handleNavigationClickCapture}>
      <RouteLoadingIndicator visible={navigationIntent} />
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50 focus:rounded-lg focus:bg-primary focus:px-4 focus:py-2 focus:text-white"
      >
        跳转到主内容
      </a>
      <AppSidebar collapsed={collapsed} />
      <div
        className={cn(
          "min-h-screen transition-[padding-left]",
          collapsed ? "md:pl-16" : "md:pl-60",
        )}
      >
        <AppHeader onToggleSidebar={() => setCollapsed((value) => !value)} />
        <main id="main-content" className="p-4 md:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
