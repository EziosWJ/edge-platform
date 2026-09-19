export type ToastType = "success" | "error" | "warning" | "info";

export type ToastOptions = {
  title?: string;
  description?: string;
  duration?: number;
};

export type ToastItem = Required<Pick<ToastOptions, "duration">> &
  Omit<ToastOptions, "duration"> & {
    id: string;
    type: ToastType;
  };

export type ToastApi = {
  show: (type: ToastType, options: string | ToastOptions) => string;
  success: (options: string | ToastOptions) => string;
  error: (options: string | ToastOptions) => string;
  warning: (options: string | ToastOptions) => string;
  info: (options: string | ToastOptions) => string;
  dismiss: (id: string) => void;
};

const DEFAULT_DURATION = 3200;

let toastItems: ToastItem[] = [];
const toastListeners = new Set<() => void>();

function normalizeToastOptions(options: string | ToastOptions): ToastOptions {
  if (typeof options === "string") {
    return { title: options };
  }

  return options;
}

function createToastId() {
  return `toast-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function emitToastChange() {
  toastListeners.forEach((listener) => listener());
}

export function subscribeToast(listener: () => void) {
  toastListeners.add(listener);
  return () => toastListeners.delete(listener);
}

export function getToastSnapshot() {
  return toastItems;
}

export function dismissToast(id: string) {
  toastItems = toastItems.filter((item) => item.id !== id);
  emitToastChange();
}

export function showToast(type: ToastType, options: string | ToastOptions) {
  const normalized = normalizeToastOptions(options);
  const id = createToastId();

  toastItems = [
    ...toastItems,
    {
      id,
      type,
      title: normalized.title,
      description: normalized.description,
      duration: normalized.duration ?? DEFAULT_DURATION,
    },
  ];
  emitToastChange();

  return id;
}

export const toast = {
  success(options: string | ToastOptions) {
    return showToast("success", options);
  },
  error(options: string | ToastOptions) {
    return showToast("error", options);
  },
  warning(options: string | ToastOptions) {
    return showToast("warning", options);
  },
  info(options: string | ToastOptions) {
    return showToast("info", options);
  },
  dismiss(id: string) {
    dismissToast(id);
  },
};
