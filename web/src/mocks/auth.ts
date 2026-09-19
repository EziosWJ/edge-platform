import type { AuthUser, LoginRequest } from "@/types";

export const mockAuthUser: AuthUser = {
  username: "admin",
  displayName: "系统管理员",
};

export function mockLogin({ username, password }: LoginRequest): AuthUser | null {
  if (username !== "admin" || password !== "admin123") {
    return null;
  }

  return {
    ...mockAuthUser,
    username,
  };
}
