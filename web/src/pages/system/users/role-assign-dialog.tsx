import { Search, ShieldCheck, X } from "lucide-react";
import { useDeferredValue, useState } from "react";
import { EmptyState } from "@/components/common/empty-state";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import type { AssignableRole, UserRecord } from "@/types";

type RoleAssignDialogProps = {
  user: UserRecord;
  roles: AssignableRole[];
  selectedRoleIds: number[];
  loading: boolean;
  submitting: boolean;
  error: string | null;
  onChange: (roleIds: number[]) => void;
  onRetry: () => void;
  onCancel: () => void;
  onSubmit: () => void;
};

export function RoleAssignDialog({
  user, roles, selectedRoleIds, loading, submitting, error,
  onChange, onRetry, onCancel, onSubmit,
}: RoleAssignDialogProps) {
  const [query, setQuery] = useState("");
  const keyword = useDeferredValue(query.trim().toLowerCase());
  const disabled = loading || submitting || !!error;
  const visibleRoles = roles.filter((role) =>
    `${role.roleName} ${role.roleCode}`.toLowerCase().includes(keyword),
  );
  const allVisibleSelected = visibleRoles.length > 0 &&
    visibleRoles.every((role) => selectedRoleIds.includes(role.id));
  const selectedRoles = selectedRoleIds.map((id) =>
    roles.find((role) => role.id === id) ?? user.roles?.find((role) => role.id === id) ??
    { id, roleName: `角色 #${id}` },
  );

  return (
    <Dialog
      open
      onOpenChange={(nextOpen) => { if (!nextOpen) onCancel(); }}
      closeOnEscape={!submitting}
      closeOnOverlayClick={false}
      trapFocus
      restoreFocus
      lockScroll
    >
      <DialogOverlay />
      <DialogContent className="max-h-[calc(100dvh-32px)] w-[calc(100%-32px)] max-w-modal-md p-0 text-text-primary">
      <div className="flex max-h-[calc(100dvh-34px)] flex-col">
        <DialogHeader className="shrink-0 gap-3 px-card py-space-4">
          <div className="min-w-0">
            <DialogTitle className="flex items-center gap-2">
              <ShieldCheck className="h-5 w-5 text-primary" aria-hidden />分配角色
            </DialogTitle>
            <DialogDescription className="text-sm">
              可选择多个角色，保存后生效。
            </DialogDescription>
          </div>
          <DialogClose disabled={submitting} aria-label="关闭角色分配">
            <X className="h-4 w-4" aria-hidden />
          </DialogClose>
        </DialogHeader>

        <DialogBody className="min-h-0 space-y-4 overflow-y-auto px-5 py-4" aria-busy={loading}>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-control bg-background px-space-3 py-space-3 text-sm">
            <span className="text-text-tertiary">分配给</span>
            <span className="break-all font-medium">{user.nickname || user.username}</span>
            {user.nickname && <span className="break-all text-text-tertiary">@{user.username}</span>}
          </div>
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-2.5 h-4 w-4 text-text-tertiary" aria-hidden />
            <Input autoFocus value={query} onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索角色名称或编码" aria-label="搜索角色名称或编码" className="pl-9 pr-10" disabled={submitting} />
            {query && <Button size="icon" variant="ghost" className="absolute right-0 top-0 h-control-md w-9"
              disabled={submitting} onClick={() => setQuery("")} aria-label="清除搜索">
              <X className="h-4 w-4" aria-hidden />
            </Button>}
          </div>

          {loading ? (
            <div role="status" className="space-y-2">
              <span className="sr-only">正在加载角色</span>
              {Array.from({ length: 4 }).map((_, index) =>
                <div key={index} className="h-16 rounded-control bg-background motion-safe:animate-pulse" />,
              )}
            </div>
          ) : error ? (
            <EmptyState title="角色数据加载失败" description={error} actionText="重新加载" onAction={onRetry} />
          ) : roles.length === 0 ? (
            <EmptyState title="暂无可分配角色" description="请先在角色管理中创建并启用角色。" />
          ) : (
            <div>
              <div className="mb-2 flex items-center justify-between gap-2 text-sm">
                <span className="text-text-tertiary">可选角色 · {visibleRoles.length}</span>
                <Button size="sm" variant="ghost" disabled={disabled || visibleRoles.length === 0}
                  onClick={() => onChange(allVisibleSelected
                    ? selectedRoleIds.filter((id) => !visibleRoles.some((role) => role.id === id))
                    : Array.from(new Set([...selectedRoleIds, ...visibleRoles.map((role) => role.id)])))}>
                  {allVisibleSelected ? "取消全选" : keyword ? "全选搜索结果" : "全选"}
                </Button>
              </div>
              {visibleRoles.length === 0 ? (
                <EmptyState title="未找到匹配角色" description="试试其他名称或角色编码。" actionText="清除搜索" onAction={() => setQuery("")} />
              ) : <div className="grid gap-2 sm:grid-cols-2">
                {visibleRoles.map((role) => {
                  const checked = selectedRoleIds.includes(role.id);
                  return (
                    <label key={role.id} className={cn(
                      "flex min-w-0 items-start gap-3 rounded-control border p-space-3 focus-within:ring-2 focus-within:ring-primary/30",
                      checked ? "border-primary bg-primary/5" : "border-border hover:border-primary/50 hover:bg-background",
                      disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer",
                    )}>
                      <Checkbox className="mt-1 shrink-0" checked={checked} disabled={disabled}
                        onChange={(event) => onChange(event.target.checked
                          ? [...selectedRoleIds, role.id] : selectedRoleIds.filter((id) => id !== role.id))} />
                      <span className="min-w-0">
                        <span className="block break-all text-sm font-medium">{role.roleName}</span>
                        <span className="mt-1 block break-all font-mono text-xs text-text-tertiary">{role.roleCode}</span>
                      </span>
                    </label>
                  );
                })}
              </div>}
            </div>
          )}

          {!loading && !error && <section className="border-t border-border pt-3" aria-label="已选角色">
            <div className="mb-2 flex items-center justify-between gap-2">
              <span className="text-sm font-medium" role="status">已选 {selectedRoleIds.length} 个角色</span>
              <Button size="sm" variant="ghost" disabled={disabled || selectedRoleIds.length === 0} onClick={() => onChange([])}>清空已选</Button>
            </div>
            {selectedRoles.length > 0 ? <div className="flex flex-wrap gap-2">
              {selectedRoles.map((role) => <button key={role.id} type="button" disabled={disabled}
                aria-label={`移除角色 ${role.roleName}`}
                onClick={() => onChange(selectedRoleIds.filter((id) => id !== role.id))}
                className="inline-flex max-w-full items-center gap-1 rounded-tag bg-primary/5 px-space-2 py-1 text-sm text-primary hover:bg-primary/10 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary disabled:cursor-not-allowed disabled:opacity-60">
                <span className="break-all">{role.roleName}</span><X className="h-3.5 w-3.5 shrink-0" aria-hidden />
              </button>)}
            </div> : <p className="text-sm text-text-tertiary">尚未选择角色，保存后该用户将不再关联任何角色。</p>}
          </section>}
        </DialogBody>

        <DialogFooter className="shrink-0 px-5 py-4">
          <Button variant="secondary" disabled={submitting} onClick={onCancel}>取消</Button>
          <Button variant="primary" disabled={disabled} onClick={onSubmit}>
            {submitting ? "保存中..." : "保存分配"}
          </Button>
        </DialogFooter>
      </div>
      </DialogContent>
    </Dialog>
  );
}
