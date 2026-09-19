import { http } from "@/lib/http";
import type {
  CurrentUser,
  CurrentUserMenu,
  LoginRequest,
  LoginResponse,
} from "@/types";

export function loginByPassword(request: LoginRequest): Promise<LoginResponse> {
  return http.post<LoginResponse>("/api/auth/login", request);
}

export function getCurrentUser(): Promise<CurrentUser> {
  return http.get<CurrentUser>("/api/auth/me");
}

export function logoutCurrentUser(): Promise<void> {
  return http.post<void>("/api/auth/logout");
}

export function getCurrentUserMenus(): Promise<CurrentUserMenu[]> {
  return http.get<CurrentUserMenu[]>("/api/auth/menus");
}
