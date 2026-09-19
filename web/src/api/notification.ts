import { http } from "@/lib/http";
import type {
  NotificationPageQuery,
  NotificationPageResult,
  NotificationPublishRequest,
  NotificationRecord,
} from "@/types/notification";

const BASE = "/api/system/notification";
export const getNotifications = (query: NotificationPageQuery) => http.get<NotificationPageResult>(`${BASE}/page`, { query });
export const getNotification = (id: number) => http.get<NotificationRecord>(`${BASE}/${id}`);
export const getUnreadNotificationCount = () => http.get<number>(`${BASE}/unread-count`);
export const markNotificationRead = (id: number) => http.put<void>(`${BASE}/${id}/read`);
export const markAllNotificationsRead = () => http.put<void>(`${BASE}/read-all`);
export const publishNotification = (data: NotificationPublishRequest) => http.post<void>(BASE, data);
export const getAdminNotifications = (query: NotificationPageQuery) => http.get<NotificationPageResult>("/api/system/notification-admin/page", { query });
