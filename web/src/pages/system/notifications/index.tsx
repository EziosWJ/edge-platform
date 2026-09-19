import { Send, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { getAdminNotifications, publishNotification } from "@/api/notification";
import { getUserPage } from "@/api/user";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn, NotificationRecord, UserRecord } from "@/types";

export function NotificationManagePage() {
  const [title, setTitle] = useState(""); const [content, setContent] = useState(""); const [users, setUsers] = useState<UserRecord[]>([]); const [selected, setSelected] = useState<number[]>([]); const [all, setAll] = useState(true); const [records, setRecords] = useState<NotificationRecord[]>([]); const [total, setTotal] = useState(0); const [page, setPage] = useState(1); const [loading, setLoading] = useState(false); const [publishing, setPublishing] = useState(false);
  const load = useCallback(async () => { setLoading(true); try { const [people, history] = await Promise.all([getUserPage({ page: 1, pageSize: 500, status: 1 }), getAdminNotifications({ page, pageSize: 10 })]); setUsers(people.records); setRecords(history.records); setTotal(history.total); } catch (error) { toast.error({ title: "通知管理加载失败", description: getErrorMessage(error, "请稍后重试") }); } finally { setLoading(false); } }, [page]);
  useEffect(() => { void load(); }, [load]);
  const publish = async () => { if (!title.trim() || !content.trim()) { toast.error("请填写标题和正文"); return; } if (!all && selected.length === 0) { toast.error("请选择接收用户"); return; } setPublishing(true); try { await publishNotification({ title, content, userIds: all ? [] : selected, allUsers: all }); setTitle(""); setContent(""); setSelected([]); toast.success("通知已发布"); await load(); } catch (error) { toast.error({ title: "发布失败", description: getErrorMessage(error, "请稍后重试") }); } finally { setPublishing(false); } };
  const columns: DataTableColumn<NotificationRecord>[] = [{ title: "标题", dataIndex: "title" }, { title: "来源", dataIndex: "sourceType", render: (value) => value === "ROLE_CHANGE" ? "角色变更" : "管理员发布" }, { title: "接收/已读", key: "read", render: (_, item) => `${item.recipientCount ?? 0} / ${item.readCount ?? 0}` }, { title: "发布时间", dataIndex: "publishTime", render: (value) => formatDateTime(String(value)) }];
  return <div><PageHeader title="通知管理" description="向启用用户发布站内通知" actions={<Button variant="secondary" size="sm" onClick={() => void load()}><RefreshCw className="h-4 w-4" />刷新</Button>} /><div className="mb-6 rounded-admin border border-border bg-surface p-5"><h2 className="mb-4 text-base font-semibold text-text-primary">发布通知</h2><div className="grid gap-4"><Input value={title} onChange={(event) => setTitle(event.target.value)} placeholder="通知标题" maxLength={200} /><Textarea value={content} onChange={(event) => setContent(event.target.value)} placeholder="纯文本正文" rows={5} maxLength={5000} /><label className="flex items-center gap-2 text-sm text-text-secondary"><input type="checkbox" checked={all} onChange={(event) => setAll(event.target.checked)} />发送给发布时全部启用用户</label>{!all && <div className="grid max-h-44 gap-2 overflow-auto rounded-lg border border-border p-3 sm:grid-cols-2">{users.map((user) => <label key={user.id} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={selected.includes(user.id)} onChange={(event) => setSelected((current) => event.target.checked ? [...current, user.id] : current.filter((id) => id !== user.id))} /><span>{user.nickname}（{user.username}）</span></label>)}</div>}<div><Button variant="primary" disabled={publishing} onClick={() => void publish()}><Send className="h-4 w-4" />{publishing ? "发布中…" : "立即发布"}</Button></div></div></div><DataTableCard className="overflow-hidden shadow-none"><DataTable columns={columns} dataSource={records} rowKey="id" loading={loading} /><Pagination page={page} pageSize={10} total={total} onPageChange={setPage} /></DataTableCard></div>;
}
