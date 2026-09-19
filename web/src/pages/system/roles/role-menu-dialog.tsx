import { X } from "lucide-react";
import { StatusTag } from "@/components/common/status-tag";
import {
  TreeCheckList,
  type TreeCheckNode,
} from "@/components/common/tree-check-list";
import { Button } from "@/components/ui/button";
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
import type { RoleDetailRecord } from "@/types";

type RoleMenuDialogProps = {
  role: RoleDetailRecord | null;
  nodes: TreeCheckNode[];
  checkedIds: number[];
  loading: boolean;
  submitting: boolean;
  onCheckedChange: (checkedIds: Array<string | number>) => void;
  onCancel: () => void;
  onSubmit: () => void;
};

export function RoleMenuDialog({
  role,
  nodes,
  checkedIds,
  loading,
  submitting,
  onCheckedChange,
  onCancel,
  onSubmit,
}: RoleMenuDialogProps) {
  if (!role) return null;

  return (
    <Dialog
      open={Boolean(role)}
      onOpenChange={(nextOpen) => { if (!nextOpen) onCancel(); }}
      closeOnEscape={false}
      closeOnOverlayClick={false}
    >
      <DialogOverlay />
      <DialogContent className="max-w-[720px]">
        <DialogHeader className="px-5 py-4">
          <div>
            <DialogTitle>
              分配菜单
            </DialogTitle>
            <DialogDescription>
              {role.roleName} / {role.roleCode}
            </DialogDescription>
          </div>
          <DialogClose
            disabled={loading || submitting}
            aria-label="关闭菜单分配"
          >
            <X className="h-4 w-4" aria-hidden />
          </DialogClose>
        </DialogHeader>

        <DialogBody className="max-h-[calc(100vh-180px)] px-5 py-5">
          <div className="mb-3 flex items-center justify-between gap-3 text-sm">
            <span className="text-text-secondary">
              已选择{" "}
              <span className="font-medium text-text-primary">
                {checkedIds.length}
              </span>{" "}
              个菜单节点
            </span>
            <StatusTag tone={loading ? "warning" : "info"}>
              {loading ? "加载中" : "菜单树"}
            </StatusTag>
          </div>

          {loading ? (
            <div className="space-y-3">
              <div className="h-control-md animate-pulse rounded-control bg-neutral-background" />
              <div className="h-control-md animate-pulse rounded-control bg-neutral-background" />
              <div className="h-control-md animate-pulse rounded-control bg-neutral-background" />
            </div>
          ) : (
            <TreeCheckList
              nodes={nodes}
              checkedIds={checkedIds}
              disabled={submitting}
              cascade
              onCheckedChange={onCheckedChange}
            />
          )}
        </DialogBody>

        <DialogFooter className="px-5 py-4">
          <Button
            variant="secondary"
            disabled={loading || submitting}
            onClick={onCancel}
          >
            取消
          </Button>
          <Button
            variant="primary"
            disabled={loading || submitting}
            onClick={onSubmit}
          >
            {submitting ? "保存中..." : "保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
