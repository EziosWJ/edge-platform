import { CheckCheck, Eye, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { getNotification, getNotifications, markAllNotificationsRead, markNotificationRead } from "@/api/notification";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogHeader, DialogOverlay, DialogTitle } from "@/components/ui/dialog";
import { formatDateTime } from "@/lib/datetime";
import { getErrorMessage } from "@/lib/api-error";
import type { DataTableColumn, NotificationRecord } from "@/types";

export function NotificationsPage() {
  const navigate = useNavigate();
  const { id } = useParams();
  const [records, setRecords] = useState<NotificationRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [detail, setDetail] = useState<NotificationRecord | null>(null);
  const load = useCallback(async () => { setLoading(true); try { const result = await getNotifications({ page, pageSize: 10 }); setRecords(result.records); setTotal(result.total); } catch (error) { toast.error({ title: "通知加载失败", description: getErrorMessage(error, "请稍后重试") }); } finally { setLoading(false); } }, [page]);
  useEffect(() => { void load(); }, [load]);
  useEffect(() => { if (!id) { setDetail(null); return; } void (async () => { try { const value = await getNotification(Number(id)); setDetail(value); if (!value.isRead) { await markNotificationRead(value.id); setRecords((current) => current.map((item) => item.id === value.id ? { ...item, isRead: 1 } : item)); } } catch (error) { toast.error({ title: "通知详情加载失败", description: getErrorMessage(error, "请稍后重试") }); navigate("/notifications", { replace: true }); } })(); }, [id, navigate]);
  const readAll = async () => { try { await markAllNotificationsRead(); setRecords((current) => current.map((item) => ({ ...item, isRead: 1 }))); toast.success("已全部标记为已读"); } catch (error) { toast.error({ title: "操作失败", description: getErrorMessage(error, "请稍后重试") }); } };
  const columns: DataTableColumn<NotificationRecord>[] = [
    { title: "标题", key: "title", render: (_, item) => <div className={item.isRead ? "text-text-secondary" : "font-semibold text-text-primary"}>{item.title}</div> },
    { title: "来源", dataIndex: "sourceType", width: 120, render: (value) => value === "ROLE_CHANGE" ? "角色变更" : "管理员发布" },
    { title: "时间", dataIndex: "publishTime", width: 180, render: (value) => formatDateTime(String(value)) },
    { title: "状态", dataIndex: "isRead", width: 90, render: (value) => value ? "已读" : "未读" },
    { title: "操作", key: "actions", align: "center", nowrap: true, width: 120, render: (_, item) => <Button size="sm" variant="ghost" onClick={() => navigate(`/notifications/${item.id}`)}><Eye className="h-4 w-4" />查看</Button> },
  ];
  return (
    <div>
      <PageHeader title="我的通知" description="查看发给你的站内通知" actions={<><Button variant="secondary" size="sm" onClick={() => void load()}><RefreshCw className="h-4 w-4" />刷新</Button><Button variant="secondary" size="sm" onClick={() => void readAll()}><CheckCheck className="h-4 w-4" />全部已读</Button></>} />
      <DataTableCard className="overflow-hidden shadow-none">
        <DataTable columns={columns} dataSource={records} rowKey="id" loading={loading} empty={<EmptyState title="暂无通知" description="你还没有收到站内通知。" />} />
        <Pagination page={page} pageSize={10} total={total} onPageChange={setPage} />
      </DataTableCard>
      {detail && (
        <Dialog
          open
          onOpenChange={(nextOpen) => { if (!nextOpen) navigate("/notifications"); }}
          closeOnEscape={false}
          closeOnOverlayClick={false}
        >
          <DialogOverlay />
          <DialogContent className="max-w-2xl p-6">
            <DialogHeader className="gap-4 border-0 p-0">
              <div>
                <DialogTitle className="text-xl">{detail.title}</DialogTitle>
                <DialogDescription className="mt-1 text-xs">
                  {formatDateTime(detail.publishTime)} · {detail.sourceType === "ROLE_CHANGE" ? "角色变更" : "管理员发布"}
                </DialogDescription>
              </div>
              <Button variant="ghost" onClick={() => navigate("/notifications")}>关闭</Button>
            </DialogHeader>
            <DialogBody className="mt-6 max-h-none overflow-visible p-0">
              <p className="whitespace-pre-wrap text-sm leading-7 text-text-secondary">{detail.content}</p>
            </DialogBody>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}
