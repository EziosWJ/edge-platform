import { ArrowDown, ArrowUp, CornerDownLeft, ExternalLink, Search, X } from "lucide-react";
import { useDeferredValue, useEffect, useId, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { convertUserMenusToNavItems, defaultNavItems, mergeNavItems } from "@/config/navigation";
import { buildMenuSearchIndex, searchMenus, type MenuSearchItem } from "@/lib/menu-search";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth-store";

export default function MenuSearchDialog({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate();
  const location = useLocation();
  const menus = useAuthStore((state) => state.menus);
  const loading = useAuthStore((state) => state.isLoadingMenus);
  const [query, setQuery] = useState("");
  const deferredQuery = useDeferredValue(query);
  const [activePath, setActivePath] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const composingRef = useRef(false);
  const id = useId();
  const index = useMemo(() => buildMenuSearchIndex(
    mergeNavItems(defaultNavItems, convertUserMenusToNavItems(menus)),
  ), [menus]);
  const results = useMemo(() => searchMenus(index, deferredQuery), [index, deferredQuery]);
  const activeIndex = Math.max(0, results.findIndex((item) => item.path === activePath));
  const activeItem = results[activeIndex];
  const pending = query !== deferredQuery;

  useEffect(() => {
    document.getElementById(`${id}-option-${activeIndex}`)?.scrollIntoView({ block: "nearest" });
  }, [activeIndex, results, id]);

  const openItem = (item: MenuSearchItem) => {
    if (pending || composingRef.current) return;
    if (item.externalUrl) {
      window.open(item.externalUrl, "_blank", "noopener,noreferrer");
    } else if (item.path !== `${location.pathname}${location.search}${location.hash}`) {
      navigate(item.path);
    }
    onClose();
  };

  return (
    <Dialog
      open
      onOpenChange={(nextOpen) => { if (!nextOpen) onClose(); }}
      trapFocus
      restoreFocus
      lockScroll
      onEscapeKeyDown={(event) => {
        event.preventDefault();
        if (!composingRef.current) onClose();
      }}
    >
      <DialogOverlay />
      <DialogContent className="max-h-[calc(100dvh-32px)] w-[calc(100%-32px)] max-w-modal-md p-0 text-text-primary">
      <div className="flex max-h-[calc(100dvh-34px)] flex-col">
        <DialogHeader className="block shrink-0 gap-0 px-4 py-4 sm:px-5">
          <div className="mb-3 flex items-center justify-between gap-3">
            <DialogTitle>菜单搜索</DialogTitle>
            <DialogClose aria-label="关闭菜单搜索" className="focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary">
              <X className="h-4 w-4" aria-hidden />
            </DialogClose>
          </div>
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-2.5 h-4 w-4 text-text-tertiary" aria-hidden />
            <Input
              ref={inputRef}
              autoFocus
              role="combobox"
              aria-label="搜索菜单"
              aria-autocomplete="list"
              aria-expanded="true"
              aria-controls={`${id}-results`}
              aria-activedescendant={activeItem ? `${id}-option-${activeIndex}` : undefined}
              aria-describedby={`${id}-hint`}
              autoComplete="off"
              spellCheck={false}
              placeholder="输入菜单名称、拼音或首字母"
              className="pl-9"
              value={query}
              onChange={(event) => { setQuery(event.target.value); setActivePath(null); }}
              onCompositionStart={() => { composingRef.current = true; }}
              onCompositionEnd={() => { composingRef.current = false; }}
              onKeyDown={(event) => {
                if (composingRef.current || event.nativeEvent.isComposing || event.keyCode === 229) return;
                if (event.key === "Enter") {
                  event.preventDefault();
                  if (activeItem) openItem(activeItem);
                } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                  event.preventDefault();
                  if (!results.length || pending) return;
                  const offset = event.key === "ArrowDown" ? 1 : -1;
                  setActivePath(results[(activeIndex + offset + results.length) % results.length].path);
                }
              }}
            />
          </div>
          <DialogDescription id={`${id}-hint`} className="mt-space-2 text-xs">搜索当前导航中的页面，也可输入所属菜单名称。</DialogDescription>
        </DialogHeader>

        <div className="flex shrink-0 items-center justify-between gap-2 px-4 pb-2 pt-3 text-xs text-text-tertiary sm:px-5" role="status">
          <span>{deferredQuery.trim() ? `找到 ${results.length} 个页面` : `全部页面 · ${results.length}`}</span>
          {loading && <span>正在加载更多菜单…</span>}
        </div>
        <DialogBody className="min-h-0 overflow-y-auto px-2 py-0 pb-2" aria-busy={pending}>
          <ul id={`${id}-results`} role="listbox" aria-label="菜单搜索结果" className="space-y-1">
            {results.map((item, position) => {
              const Icon = item.icon;
              const current = !item.externalUrl && item.path === location.pathname;
              return (
                <li key={item.path} role="presentation">
                  <button
                    id={`${id}-option-${position}`}
                    type="button"
                    role="option"
                    aria-selected={position === activeIndex}
                    tabIndex={-1}
                    onMouseDown={(event) => event.preventDefault()}
                    onClick={() => openItem(item)}
                    className={cn(
                      "flex w-full items-center gap-3 rounded-lg border px-3 py-3 text-left outline-none focus-visible:border-primary",
                      position === activeIndex ? "border-primary bg-blue-50" : "border-transparent hover:bg-slate-50",
                    )}
                  >
                    <Icon className={cn("h-5 w-5 shrink-0", position === activeIndex ? "text-primary" : "text-text-tertiary")} aria-hidden />
                    <span className="min-w-0 flex-1">
                      <span className="block break-words text-sm font-medium">{item.label}</span>
                      <span className="mt-1 block break-all text-xs text-text-tertiary">{item.breadcrumb || "主导航"}</span>
                    </span>
                    {item.externalUrl ? <span className="flex shrink-0 items-center gap-1 text-xs text-text-tertiary">
                      <ExternalLink className="h-3.5 w-3.5" aria-hidden /><span className="sr-only sm:not-sr-only">新标签页</span>
                    </span> : current ? <span className="shrink-0 text-xs text-primary">当前页</span>
                      : position === activeIndex && <CornerDownLeft className="h-4 w-4 shrink-0 text-primary" aria-hidden />}
                  </button>
                </li>
              );
            })}
          </ul>
          {results.length === 0 && <div className="px-4 py-10 text-center">
            <Search className="mx-auto h-7 w-7 text-text-tertiary" aria-hidden />
            <p className="mt-3 text-sm font-medium">未找到匹配菜单</p>
            <p className="mt-1 text-sm text-text-tertiary">试试其他名称、拼音或首字母。</p>
          </div>}
        </DialogBody>

        <DialogFooter className="shrink-0 flex-wrap justify-start gap-x-4 gap-y-2 px-4 py-3 text-xs text-text-tertiary sm:px-5">
          <span className="inline-flex items-center gap-1"><ArrowUp className="h-3 w-3" aria-hidden /><ArrowDown className="h-3 w-3" aria-hidden />选择</span>
          <span className="inline-flex items-center gap-1"><CornerDownLeft className="h-3 w-3" aria-hidden />跳转</span>
          <span><kbd className="font-sans">Esc</kbd> 关闭</span>
        </DialogFooter>
      </div>
      </DialogContent>
    </Dialog>
  );
}
