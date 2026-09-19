import { createContext, useContext } from "react";
import type { ToastApi } from "@/components/common/toast-store";

export const ToastContext = createContext<ToastApi | null>(null);

export function useToast() {
  const api = useContext(ToastContext);

  if (!api) {
    throw new Error("useToast must be used inside ToastProvider.");
  }

  return api;
}
