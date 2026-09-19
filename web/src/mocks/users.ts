import type { UserListQuery, UserRecord } from "@/types";

export const mockUsers: UserRecord[] = [
  {
    id: 1001,
    name: "张明",
    account: "zhangming",
    role: "管理员",
    department: "平台运营部",
    status: "active",
    lastLogin: "2026-04-28 09:24",
  },
  {
    id: 1002,
    name: "李娜",
    account: "lina",
    role: "运营",
    department: "内容运营部",
    status: "active",
    lastLogin: "2026-04-27 18:12",
  },
  {
    id: 1003,
    name: "王磊",
    account: "wanglei",
    role: "审计员",
    department: "风控中心",
    status: "pending",
    lastLogin: "2026-04-25 10:08",
  },
  {
    id: 1004,
    name: "陈静",
    account: "chenjing",
    role: "访客",
    department: "外部协作",
    status: "disabled",
    lastLogin: "2026-04-18 14:36",
  },
];

export function filterMockUsers(query: UserListQuery = {}) {
  const keyword = query.keyword?.trim();
  const status = query.status ?? "all";

  return mockUsers.filter((user) => {
    const matchedKeyword =
      !keyword ||
      user.name.includes(keyword) ||
      user.account.includes(keyword) ||
      user.department.includes(keyword);
    const matchedStatus = status === "all" || user.status === status;

    return matchedKeyword && matchedStatus;
  });
}

