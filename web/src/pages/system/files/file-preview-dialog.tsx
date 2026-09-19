import { Download, Loader2, X } from "lucide-react";
import { getFileViewUrl } from "@/api/file";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from "@/components/ui/dialog";
import { useAuthenticatedFileUrl } from "@/hooks/use-authenticated-file-url";
import type { FileRecord } from "@/types";

type FilePreviewDialogProps = {
  open: boolean;
  record: FileRecord | null;
  onCancel: () => void;
  onDownload: (record: FileRecord) => void;
};

export function FilePreviewDialog({
  open,
  record,
  onCancel,
  onDownload,
}: FilePreviewDialogProps) {
  const previewable = Boolean(
    record &&
      (record.mimeType.startsWith("image/") || record.mimeType === "application/pdf"),
  );
  const previewPath = record && previewable ? getFileViewUrl(record.id) : null;
  const {
    url: previewUrl,
    loading: previewLoading,
    error: previewError,
  } = useAuthenticatedFileUrl(previewPath);

  if (!open || !record) return null;

  const isImage = record.mimeType.startsWith("image/");
  const isPdf = record.mimeType === "application/pdf";

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => { if (!nextOpen) onCancel(); }}>
      <DialogOverlay className="bg-slate-950/50" />
      <DialogContent className="flex max-h-[calc(100vh-48px)] max-w-[960px] flex-col">
        <DialogHeader className="items-center px-5 py-3">
          <DialogTitle className="min-w-0 flex-1 truncate">
            {record.originalName}
          </DialogTitle>
          <div className="flex items-center gap-1">
            <Button size="sm" variant="secondary" onClick={() => onDownload(record)}>
              <Download className="h-4 w-4" aria-hidden />
              下载
            </Button>
            <DialogClose aria-label="关闭预览">
              <X className="h-4 w-4" aria-hidden />
            </DialogClose>
          </div>
        </DialogHeader>

        <DialogBody className="flex min-h-0 flex-1 items-center justify-center overflow-hidden bg-slate-100 p-4">
          {previewLoading && (
            <div className="flex items-center gap-2 text-sm text-text-tertiary" role="status">
              <Loader2 className="h-4 w-4 animate-spin" aria-hidden />
              正在加载预览...
            </div>
          )}
          {previewError && (
            <p className="text-sm text-error" role="alert">{previewError}</p>
          )}
          {!previewLoading && !previewError && isImage && previewUrl && (
            <img
              src={previewUrl}
              alt={record.originalName}
              className="max-h-full max-w-full object-contain"
            />
          )}
          {!previewLoading && !previewError && isPdf && previewUrl && (
            <iframe
              src={previewUrl}
              title={record.originalName}
              className="h-full w-full border-0"
            />
          )}
          {!isImage && !isPdf && (
            <div className="py-12 text-center text-sm text-text-tertiary">
              此文件类型不支持浏览器预览，请下载后查看。
            </div>
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
