import { X } from "lucide-react";
import { type FormEvent, type ReactNode } from "react";
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
import { cn } from "@/lib/utils";

type FormDialogProps = {
  open: boolean;
  title: string;
  description?: ReactNode;
  loading?: boolean;
  submitDisabled?: boolean;
  submitText?: string;
  loadingText?: string;
  cancelText?: string;
  contentClassName?: string;
  headerClassName?: string;
  bodyClassName?: string;
  footerClassName?: string;
  closeOnEscape?: boolean;
  closeOnOverlayClick?: boolean;
  trapFocus?: boolean;
  onCancel: () => void;
  onSubmit: () => void | Promise<void>;
  children: ReactNode;
};

export function FormDialog({
  open,
  title,
  description,
  loading = false,
  submitDisabled = false,
  submitText = "保存",
  loadingText = "提交中...",
  cancelText = "取消",
  contentClassName,
  headerClassName,
  bodyClassName,
  footerClassName,
  closeOnEscape = true,
  closeOnOverlayClick = true,
  trapFocus = true,
  onCancel,
  onSubmit,
  children,
}: FormDialogProps) {
  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    if (!loading) {
      void onSubmit();
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) onCancel();
      }}
      closeOnEscape={closeOnEscape && !loading}
      closeOnOverlayClick={closeOnOverlayClick && !loading}
      trapFocus={trapFocus}
    >
      <DialogOverlay />
      <DialogContent
        className={cn(
          "max-h-[calc(100vh-48px)] max-w-[760px]",
          contentClassName,
        )}
      >
        <DialogHeader className={headerClassName}>
          <div className="min-w-0">
            <DialogTitle>{title}</DialogTitle>
            {description && (
              <DialogDescription>{description}</DialogDescription>
            )}
          </div>
          <DialogClose disabled={loading} aria-label="关闭表单弹窗">
            <X className="h-4 w-4" aria-hidden />
          </DialogClose>
        </DialogHeader>

        <form onSubmit={handleSubmit}>
          <DialogBody
            className={cn(
              "max-h-[calc(100vh-184px)] px-card py-space-4",
              bodyClassName,
            )}
          >
            {children}
          </DialogBody>
          <DialogFooter className={footerClassName}>
            <Button variant="secondary" disabled={loading} onClick={onCancel}>
              {cancelText}
            </Button>
            <Button
              type="submit"
              variant="primary"
              disabled={loading || submitDisabled}
              className={cn(loading && "cursor-wait")}
            >
              {loading ? loadingText : submitText}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
