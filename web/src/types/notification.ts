export type NotificationRecord = {
  id: number;
  title: string;
  content: string;
  sourceType: "MANUAL" | "ROLE_CHANGE" | string;
  publisherId?: number | null;
  publishTime: string;
  createTime: string;
  isRead: 0 | 1;
  recipientCount?: number;
  readCount?: number;
};

export type NotificationPageQuery = { page: number; pageSize: number };
export type NotificationPageResult = {
  records: NotificationRecord[];
  total: number;
  page: number;
  pageSize: number;
};
export type NotificationPublishRequest = {
  title: string;
  content: string;
  userIds: number[];
  allUsers?: boolean;
};
