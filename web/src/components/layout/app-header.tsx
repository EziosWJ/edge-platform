import { Menu, Search } from "lucide-react";
import { lazy, Suspense, useEffect, useState } from "react";
import { Breadcrumbs } from "@/components/layout/breadcrumbs";
import { UserMenu } from "@/components/layout/user-menu";
import { NotificationBell } from "@/components/layout/notification-bell";
import { Button } from "@/components/ui/button";

const MenuSearchDialog = lazy(() => import("./menu-search-dialog"));

type AppHeaderProps = {
  onToggleSidebar: () => void;
};

export function AppHeader({ onToggleSidebar }: AppHeaderProps) {
  const [searchOpen, setSearchOpen] = useState(false);
  const shortcut = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘ K" : "Ctrl K";

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.isComposing || event.repeat || event.altKey ||
        !(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== "k") return;
      if (!searchOpen && document.querySelector('[aria-modal="true"], dialog[open]')) return;
      event.preventDefault();
      setSearchOpen((open) => !open);
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [searchOpen]);

  return (
    <header className="sticky top-0 z-20 flex h-16 items-center justify-between border-b border-border bg-surface px-4 md:px-6">
      <div className="flex min-w-0 items-center gap-3">
        <Button
          variant="ghost"
          size="icon"
          aria-label="折叠侧边栏"
          title="折叠侧边栏"
          onClick={onToggleSidebar}
        >
          <Menu className="h-5 w-5" aria-hidden />
        </Button>
        <div className="hidden md:block">
          <Breadcrumbs />
        </div>
      </div>

      <div className="flex items-center gap-3">
        <button
          type="button"
          className="hidden h-9 w-64 items-center gap-2 rounded-lg border border-border bg-slate-50 px-3 text-left text-sm text-text-tertiary transition-colors hover:border-slate-300 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary md:flex"
          aria-label="菜单搜索"
          aria-haspopup="dialog"
          aria-expanded={searchOpen}
          aria-keyshortcuts="Control+k Meta+k"
          onClick={() => setSearchOpen(true)}
        >
          <Search className="h-4 w-4" aria-hidden />
          搜索菜单
          <kbd className="ml-auto rounded border border-border bg-surface px-1.5 py-0.5 font-sans text-xs">{shortcut}</kbd>
        </button>
        <Button
          variant="ghost"
          size="icon"
          className="focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary md:hidden"
          aria-label="菜单搜索"
          title="菜单搜索"
          aria-haspopup="dialog"
          aria-expanded={searchOpen}
          onClick={() => setSearchOpen(true)}
        >
          <Search className="h-4 w-4" aria-hidden />
        </Button>
        <NotificationBell />
        <UserMenu />
      </div>
      {searchOpen && (
        <Suspense fallback={<span role="status" className="fixed right-4 top-20 rounded-lg border border-border bg-surface px-4 py-3 text-sm shadow-admin">正在加载菜单搜索…</span>}>
          <MenuSearchDialog onClose={() => setSearchOpen(false)} />
        </Suspense>
      )}
    </header>
  );
}
