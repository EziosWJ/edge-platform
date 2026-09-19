import {
  AlertCircle,
  CheckCircle2,
  Info,
  TriangleAlert,
  X,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useMemo,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { ToastContext } from "@/components/common/use-toast";
import { Button } from "@/components/ui/button";
import {
  dismissToast,
  getToastSnapshot,
  showToast,
  subscribeToast,
  type ToastApi,
  type ToastItem,
  type ToastOptions,
  type ToastType,
} from "@/components/common/toast-store";
import { cn } from "@/lib/utils";

const icons: Record<ToastType, ReactNode> = {
  success: <CheckCircle2 className="h-4 w-4 text-success" aria-hidden />,
  error: <AlertCircle className="h-4 w-4 text-error" aria-hidden />,
  warning: <TriangleAlert className="h-4 w-4 text-warning" aria-hidden />,
  info: <Info className="h-4 w-4 text-info" aria-hidden />,
};

export function ToastProvider({ children }: { children: ReactNode }) {
  const dismiss = useCallback((id: string) => dismissToast(id), []);
  const show = useCallback(
    (type: ToastType, options: string | ToastOptions) =>
      showToast(type, options),
    [],
  );

  const api = useMemo<ToastApi>(
    () => ({
      show,
      success: (options) => show("success", options),
      error: (options) => show("error", options),
      warning: (options) => show("warning", options),
      info: (options) => show("info", options),
      dismiss,
    }),
    [dismiss, show],
  );

  return (
    <ToastContext.Provider value={api}>
      {children}
      <Toaster />
    </ToastContext.Provider>
  );
}

export function Toaster() {
  const items = useSyncExternalStore(
    subscribeToast,
    getToastSnapshot,
    getToastSnapshot,
  );

  return <ToastViewport items={items} onDismiss={dismissToast} />;
}

function ToastViewport({
  items,
  onDismiss,
}: {
  items: ToastItem[];
  onDismiss: (id: string) => void;
}) {
  if (typeof document === "undefined") {
    return null;
  }

  return createPortal(
    <div
      className="fixed right-space-4 top-space-4 z-50 flex w-[min(360px,calc(100vw-32px))] flex-col gap-space-2"
      role="region"
      aria-label="消息通知"
    >
      {items.map((item) => (
        <ToastMessage key={item.id} item={item} onDismiss={onDismiss} />
      ))}
    </div>,
    document.body,
  );
}

function ToastMessage({
  item,
  onDismiss,
}: {
  item: ToastItem;
  onDismiss: (id: string) => void;
}) {
  useEffect(() => {
    if (item.duration <= 0) {
      return;
    }

    const timer = window.setTimeout(() => onDismiss(item.id), item.duration);
    return () => window.clearTimeout(timer);
  }, [item.duration, item.id, onDismiss]);

  return (
    <div
      className="flex gap-space-3 rounded-admin border border-border bg-surface p-space-3 text-sm shadow-admin"
      role={item.type === "error" ? "alert" : "status"}
    >
      <div className="mt-0.5 shrink-0">{icons[item.type]}</div>
      <div className="min-w-0 flex-1">
        {item.title && (
          <div className="break-words font-medium text-text-primary">
            {item.title}
          </div>
        )}
        {item.description && (
          <div
            className={cn(
              "break-words text-body-secondary text-text-tertiary",
              item.title && "mt-0.5",
            )}
          >
            {item.description}
          </div>
        )}
      </div>
      <Button
        size="icon"
        variant="ghost"
        className="h-7 w-7 shrink-0"
        onClick={() => onDismiss(item.id)}
        aria-label="关闭通知"
      >
        <X className="h-4 w-4" aria-hidden />
      </Button>
    </div>
  );
}
