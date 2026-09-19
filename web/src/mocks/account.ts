import type { AccountProfile } from "@/types";

export const mockAccountProfile: AccountProfile = {
  username: "admin",
  displayName: "系统管理员",
  email: "admin@example.com",
  phone: "13800000000",
  department: "平台运营部",
  role: "超级管理员",
  roleCode: "super_admin",
  lastLogin: "2026-05-02 09:32",
  status: "enabled",
};

export function mockGetAccountProfile(): AccountProfile {
  return mockAccountProfile;
}

