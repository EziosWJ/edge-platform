import { create } from "zustand";
import {
  getCurrentUser,
  getCurrentUserMenus,
  loginByPassword,
  logoutCurrentUser,
} from "@/api/auth";
import { setAuthTokenGetter, setUnauthorizedHandler } from "@/lib/http";
import type { AuthState, CurrentUser } from "@/types";

const AUTH_STORAGE_KEY = "web-auth";
let currentUserPromise: Promise<CurrentUser> | null = null;

type StoredAuth = {
  token: string;
  user: CurrentUser | null;
};

function isStoredAuth(value: unknown): value is StoredAuth {
  if (typeof value !== "object" || value === null) return false;

  const auth = value as Partial<StoredAuth>;
  return typeof auth.token === "string";
}

function readStoredAuth(): StoredAuth {
  const fallback: StoredAuth = { token: "", user: null };
  const raw = localStorage.getItem(AUTH_STORAGE_KEY);
  if (!raw) return fallback;

  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!isStoredAuth(parsed)) {
      localStorage.removeItem(AUTH_STORAGE_KEY);
      return fallback;
    }

    return {
      token: parsed.token,
      user: parsed.user ?? null,
    };
  } catch {
    localStorage.removeItem(AUTH_STORAGE_KEY);
    return fallback;
  }
}

function writeStoredAuth(token: string, user: CurrentUser | null) {
  localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify({ token, user }));
}

function clearStoredAuth() {
  localStorage.removeItem(AUTH_STORAGE_KEY);
}

function redirectToLogin() {
  if (window.location.pathname === "/login") return;

  const next = `${window.location.pathname}${window.location.search}`;
  window.location.replace(`/login?redirect=${encodeURIComponent(next)}`);
}

const storedAuth = readStoredAuth();

export const useAuthStore = create<AuthState>((set, get) => ({
  token: storedAuth.token || null,
  user: storedAuth.user,
  menus: [],
  isAuthenticated: Boolean(storedAuth.token),
  isLoadingUser: false,
  isLoadingMenus: false,

  login: async (username, password) => {
    const loginResult = await loginByPassword({ username, password });
    const token = loginResult.tokenValue;

    set({
      token,
      user: null,
      menus: [],
      isAuthenticated: true,
      isLoadingUser: true,
      isLoadingMenus: false,
    });
    writeStoredAuth(token, null);

    try {
      const user = await getCurrentUser();
      writeStoredAuth(token, user);
      set({ user, isAuthenticated: true, isLoadingUser: false });
      void get().fetchCurrentUserMenus(true).catch(() => undefined);
      return user;
    } catch (error) {
      get().clearAuth();
      throw error;
    }
  },

  fetchCurrentUser: async () => {
    const { token, user, isLoadingUser } = get();
    if (!token) return null;
    if (user) return user;
    if (isLoadingUser && currentUserPromise) return currentUserPromise;

    set({ isLoadingUser: true });

    currentUserPromise = (async () => {
      try {
        const nextUser = await getCurrentUser();
        writeStoredAuth(token, nextUser);
        set({
          user: nextUser,
          isAuthenticated: true,
          isLoadingUser: false,
        });
        void get().fetchCurrentUserMenus();
        return nextUser;
      } catch (error) {
        get().clearAuth();
        throw error;
      } finally {
        currentUserPromise = null;
      }
    })();

    return currentUserPromise;
  },

  fetchCurrentUserMenus: async (force = false) => {
    const { token, menus, isLoadingMenus } = get();
    if (!token) return [];
    if (!force && menus.length > 0) return menus;
    if (isLoadingMenus) return menus;

    set({ isLoadingMenus: true });

    try {
      const nextMenus = await getCurrentUserMenus();
      set({ menus: nextMenus, isLoadingMenus: false });
      return nextMenus;
    } catch (error) {
      set({ isLoadingMenus: false });
      throw error;
    }
  },

  logout: async () => {
    try {
      if (get().token) {
        setUnauthorizedHandler(null);
        await logoutCurrentUser();
      }
    } catch {
      // 退出登录以本地清理为准，接口失败不阻塞用户离开当前会话。
    } finally {
      setUnauthorizedHandler(() => {
        useAuthStore.getState().clearAuth();
        redirectToLogin();
      });
      get().clearAuth();
    }
  },

  clearAuth: () => {
    clearStoredAuth();
    set({
      token: null,
      user: null,
      menus: [],
      isAuthenticated: false,
      isLoadingUser: false,
      isLoadingMenus: false,
    });
  },
}));

setAuthTokenGetter(() => useAuthStore.getState().token);
setUnauthorizedHandler(() => {
  useAuthStore.getState().clearAuth();
  redirectToLogin();
});
