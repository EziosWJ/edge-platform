import type { PropsWithChildren } from "react";
import { useEffect } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { useAuthStore } from "@/store/auth-store";

export function RequireAuth({ children }: PropsWithChildren) {
  const location = useLocation();
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const user = useAuthStore((state) => state.user);
  const isLoadingUser = useAuthStore((state) => state.isLoadingUser);
  const fetchCurrentUser = useAuthStore((state) => state.fetchCurrentUser);

  useEffect(() => {
    if (isAuthenticated && !user && !isLoadingUser) {
      void fetchCurrentUser().catch(() => undefined);
    }
  }, [fetchCurrentUser, isAuthenticated, isLoadingUser, user]);

  if (!isAuthenticated) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  if (!user) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-background px-4 text-sm text-text-tertiary">
        正在加载用户信息...
      </main>
    );
  }

  return children;
}
