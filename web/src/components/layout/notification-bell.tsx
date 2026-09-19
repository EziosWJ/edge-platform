import { Bell, CheckCheck } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { getNotifications, getUnreadNotificationCount, markAllNotificationsRead } from "@/api/notification";
import { Button } from "@/components/ui/button";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import { toast } from "@/components/common/toast-store";
import type { NotificationRecord } from "@/types";

export function NotificationBell() {
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [count, setCount] = useState(0);
  const [items, setItems] = useState<NotificationRecord[]>([]);
  const rootRef = useRef<HTMLDivElement | null>(null);

  const load = async () => {
    try {
      const [unread, page] = await Promise.all([
        getUnreadNotificationCount(),
        getNotifications({ page: 1, pageSize: 5 }),
      ]);
      setCount(unread);
      setItems(page.records);
    } catch {
      // 通知中心不应阻塞主页面，完整错误在通知页展示。
    }
  };

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") void load();
    }, 30_000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    const close = (event: MouseEvent) => {
      if (rootRef.current && event.target instanceof Node && !rootRef.current.contains(event.target)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, []);

  const readAll = async () => {
    try {
      await markAllNotificationsRead();
      setCount(0);
      setItems((current) => current.map((item) => ({ ...item, isRead: 1 })));
    } catch (error) {
      toast.error({ title: "操作失败", description: getErrorMessage(error, "请稍后重试") });
    }
  };

  return (
    <div ref={rootRef} className="relative">
      <Button variant="ghost" size="icon" aria-label="站内通知" title="站内通知" aria-expanded={open} onClick={() => setOpen((value) => !value)}>
        <Bell className="h-4 w-4" aria-hidden />
        {count > 0 && <span className="absolute right-0 top-0 min-w-4 rounded-full bg-error px-1 text-center text-[10px] leading-4 text-white">{count > 99 ? "99+" : count}</span>}
      </Button>
      {open && (
        <div className="absolute right-0 top-[calc(100%+8px)] z-40 w-80 rounded-admin border border-border bg-surface p-3 shadow-admin">
          <div className="mb-2 flex items-center justify-between"><span className="font-medium text-text-primary">最近通知</span><button type="button" className="text-xs text-primary hover:underline" onClick={() => void readAll()}><CheckCheck className="mr-1 inline h-3.5 w-3.5" />全部已读</button></div>
          {items.length === 0 ? <p className="py-6 text-center text-sm text-text-tertiary">暂无通知</p> : <div className="divide-y divide-border">{items.map((item) => <button key={item.id} type="button" className="block w-full py-2 text-left hover:bg-slate-50" onClick={() => { setOpen(false); navigate(`/notifications/${item.id}`); }}><div className={`truncate text-sm ${item.isRead ? "text-text-secondary" : "font-semibold text-text-primary"}`}>{item.title}</div><div className="mt-1 text-xs text-text-tertiary">{formatDateTime(item.publishTime)}</div></button>)}</div>}
          <button type="button" className="mt-2 w-full border-t border-border pt-2 text-center text-sm text-primary hover:underline" onClick={() => { setOpen(false); navigate("/notifications"); }}>查看全部</button>
        </div>
      )}
    </div>
  );
}
