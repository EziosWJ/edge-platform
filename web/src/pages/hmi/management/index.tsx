import { FileEdit, Plus, RefreshCw, Search } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { PermissionGuard } from "@/components/auth/permission-guard";
import { DataTable } from "@/components/common/data-table";
import { DataTableCard } from "@/components/common/data-table-card";
import { EmptyState } from "@/components/common/empty-state";
import { Field } from "@/components/common/field";
import { FormDialog } from "@/components/common/form-dialog";
import { PageHeader } from "@/components/common/page-header";
import { Pagination } from "@/components/common/pagination";
import { SearchFilterBar } from "@/components/common/search-filter-bar";
import { TableToolbar } from "@/components/common/table-toolbar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { toast } from "@/components/common/toast-store";
import { getErrorMessage } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import type { DataTableColumn } from "@/types";
import { createHmiPage, getHmiPagePage, updateHmiPage, type HmiPageMetadataInput, type HmiPageRecord } from "@/hmi/editor/api";

type MetadataDialogState = { mode: "create" } | { mode: "edit"; page: HmiPageRecord } | null;
const PAGE_SIZE = 20;

export function HmiPagesPage() {
  const navigate = useNavigate();
  const [records, setRecords] = useState<HmiPageRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [nameFilter, setNameFilter] = useState("");
  const [appliedNameFilter, setAppliedNameFilter] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [dialog, setDialog] = useState<MetadataDialogState>(null);
  const [form, setForm] = useState<HmiPageMetadataInput>({ name: "", description: "" });
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = await getHmiPagePage({ page, pageSize: PAGE_SIZE, name: appliedNameFilter || undefined });
      setRecords(result.records);
      setTotal(result.total);
    } catch (loadError) {
      setRecords([]);
      setTotal(0);
      setError(getErrorMessage(loadError, "HMI 页面列表加载失败"));
    } finally {
      setLoading(false);
    }
  }, [appliedNameFilter, page]);

  useEffect(() => { void load(); }, [load]);

  const columns = useMemo<DataTableColumn<HmiPageRecord>[]>(() => [
    { title: "页面名称", dataIndex: "name", render: (value, record) => <button className="text-left font-medium text-primary hover:underline" onClick={() => navigate(`/hmi/pages/${encodeURIComponent(record.pageId)}/edit`)}>{String(value ?? "未命名页面")}</button> },
    { title: "说明", dataIndex: "description", render: (value) => <span className="text-text-secondary">{String(value || "-")}</span> },
    { title: "草稿修订", dataIndex: "draftRevision", width: 110, align: "center", nowrap: true },
    { title: "已发布", key: "published", width: 120, align: "center", render: (_, record) => record.publishedVersionId ? <span className="text-success">已发布</span> : <span className="text-text-tertiary">未发布</span> },
    { title: "更新时间", dataIndex: "updatedAt", width: 200, nowrap: true, render: (value) => <span className="tabular-nums">{formatDateTime(String(value ?? ""))}</span> },
    { title: "操作", key: "actions", width: 190, align: "center", nowrap: true, render: (_, record) => <div className="flex justify-center gap-1"><Button size="sm" variant="ghost" onClick={() => navigate(`/hmi/pages/${encodeURIComponent(record.pageId)}/edit`)}><FileEdit className="h-4 w-4" aria-hidden />编辑</Button><PermissionGuard permissionCode="hmi:edit"><Button size="sm" variant="ghost" onClick={() => openEdit(record)}>属性</Button></PermissionGuard></div> },
  ], [navigate]);

  function openCreate() {
    setForm({ name: "", description: "" });
    setDialog({ mode: "create" });
  }

  function openEdit(record: HmiPageRecord) {
    setForm({ name: record.name, description: record.description ?? "" });
    setDialog({ mode: "edit", page: record });
  }

  async function submitMetadata() {
    if (!form.name.trim()) {
      toast.error("页面名称不能为空");
      return;
    }
    setSaving(true);
    try {
      if (dialog?.mode === "create") {
        const created = await createHmiPage({ name: form.name.trim(), description: form.description.trim() });
        toast.success("HMI 页面已创建");
        setDialog(null);
        navigate(`/hmi/pages/${encodeURIComponent(created.pageId)}/edit`);
      } else if (dialog?.mode === "edit") {
        await updateHmiPage(dialog.page.pageId, { name: form.name.trim(), description: form.description.trim() });
        toast.success("页面属性已保存");
        setDialog(null);
        void load();
      }
    } catch (saveError) {
      toast.error(getErrorMessage(saveError, "保存失败"));
    } finally {
      setSaving(false);
    }
  }

  return <>
    <PageHeader title="HMI 页面" description="管理 HMI 页面草稿与发布版本。运行页面只展示已发布版本。" actions={<PermissionGuard permissionCode="hmi:edit"><Button variant="primary" onClick={openCreate}><Plus className="h-4 w-4" aria-hidden />新建页面</Button></PermissionGuard>} />
    <form onSubmit={(event) => { event.preventDefault(); setPage(1); setAppliedNameFilter(nameFilter.trim()); }}>
      <SearchFilterBar actions={<><Button type="button" variant="secondary" onClick={() => { setNameFilter(""); setAppliedNameFilter(""); setPage(1); }}><Search className="h-4 w-4" aria-hidden />重置</Button><Button type="submit" variant="primary"><Search className="h-4 w-4" aria-hidden />查询</Button></>}>
        <Input value={nameFilter} onChange={(event) => setNameFilter(event.target.value)} placeholder="按页面名称筛选" aria-label="页面名称" />
      </SearchFilterBar>
    </form>
    <DataTableCard className="mt-space-5" toolbar={<TableToolbar title="页面列表" description={`共 ${total} 个 HMI 页面`} actions={<Button variant="secondary" size="sm" onClick={() => void load()} disabled={loading}><RefreshCw className="h-4 w-4" aria-hidden />刷新</Button>} />} pagination={<Pagination page={page} pageSize={PAGE_SIZE} total={total} onPageChange={setPage} />}>
      <DataTable columns={columns} dataSource={records} rowKey="pageId" loading={loading} error={error} empty={<EmptyState title="暂无 HMI 页面" description="创建页面后，可在固定画布中添加文本、形状和绑定组件。" />} minWidth={900} />
    </DataTableCard>
    <FormDialog open={dialog !== null} title={dialog?.mode === "create" ? "新建 HMI 页面" : "编辑页面属性"} description="页面名称和说明可随时更新，不会改变已发布版本。" loading={saving} submitDisabled={!form.name.trim()} submitText="保存" onCancel={() => setDialog(null)} onSubmit={submitMetadata}>
      <div className="space-y-space-4">
        <Field label="页面名称" htmlFor="hmi-name" required><Input id="hmi-name" maxLength={100} value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} autoFocus /></Field>
        <Field label="页面说明" htmlFor="hmi-description"><Textarea id="hmi-description" maxLength={500} rows={4} value={form.description} onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))} /></Field>
      </div>
    </FormDialog>
  </>;
}
