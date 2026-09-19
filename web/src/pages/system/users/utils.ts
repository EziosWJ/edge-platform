import type { UserRecord } from "@/types";

export function getRoleNames(user: UserRecord) {
  if (!user.roles?.length) return "-";
  return user.roles.map((role) => role.roleName).join("、");
}
