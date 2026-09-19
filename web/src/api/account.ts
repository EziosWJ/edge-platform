import { http } from "@/lib/http";
import type {
  ChangePasswordRequest,
  UpdateAvatarRequest,
} from "@/types/account";
import type { CurrentUser } from "@/types/auth";

export async function getAccountProfile(): Promise<CurrentUser> {
  return http.get<CurrentUser>("/api/auth/me");
}

export function changeCurrentUserPassword(data: ChangePasswordRequest) {
  return http.put<void>("/api/system/user/me/password", data);
}

export function updateCurrentUserAvatar(data: UpdateAvatarRequest) {
  return http.put<void>("/api/system/user/me/avatar", data);
}
