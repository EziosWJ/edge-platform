import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from "@/components/ui/dialog";
import type { UserRecord } from "@/types";

type PasswordResultDialogProps = {
  result: { user: UserRecord; password: string } | null;
  onClose: () => void;
};

export function PasswordResultDialog({
  result,
  onClose,
}: PasswordResultDialogProps) {
  if (!result) return null;

  return (
    <Dialog
      open={Boolean(result)}
      onOpenChange={(nextOpen) => { if (!nextOpen) onClose(); }}
      closeOnEscape={false}
      closeOnOverlayClick={false}
    >
      <DialogOverlay />
      <DialogContent className="max-w-modal-sm">
        <DialogHeader>
          <DialogTitle>密码重置成功</DialogTitle>
          <DialogDescription className="text-sm">
            请将新密码线下通知用户「{result.user.nickname || result.user.username}」。
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="max-h-none overflow-visible px-5 py-4">
          <div className="rounded-control border border-border bg-neutral-background px-space-4 py-space-3 font-mono text-lg font-semibold tabular-nums text-text-primary">
            {String(result.password)}
          </div>
        </DialogBody>
        <DialogFooter className="border-t-0 px-5 py-4">
          <Button variant="primary" onClick={onClose}>
            知道了
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
