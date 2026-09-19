import { Upload, X } from "lucide-react";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { uploadFiles } from "@/api/file";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { DictSelectOption } from "@/constants/dicts";
import type { FileRecord, FileUploadBatchResult } from "@/types";
import { getErrorMessage } from "@/lib/api-error";

type FileUploadDialogProps = {
  open: boolean;
  businessModuleOptions: DictSelectOption[];
  onCancel: () => void;
  onUploaded: (records: FileRecord[]) => void;
};

export function FileUploadDialog({
  open,
  businessModuleOptions,
  onCancel,
  onUploaded,
}: FileUploadDialogProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [files, setFiles] = useState<File[]>([]);
  const [businessModule, setBusinessModule] = useState("");
  const [remark, setRemark] = useState("");
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<FileUploadBatchResult | null>(null);

  useEffect(() => {
    if (!open) return;

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !uploading) {
        onCancel();
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onCancel, open, uploading]);

  useEffect(() => {
    if (!open) {
      setFiles([]);
      setBusinessModule("");
      setRemark("");
      setError("");
      setResult(null);
    }
  }, [open]);

  const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setFiles(Array.from(event.target.files ?? []));
    setError("");
    setResult(null);
  };

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (files.length === 0) {
      setError("请选择至少一个文件");
      return;
    }

    setUploading(true);
    setError("");

    try {
      const uploadResult = await uploadFiles(files, {
        businessModule: businessModule.trim() || undefined,
        remark: remark.trim() || undefined,
      });
      const mergedResult: FileUploadBatchResult = {
        succeeded: [...(result?.succeeded ?? []), ...uploadResult.succeeded],
        failed: uploadResult.failed,
      };
      setResult(mergedResult);
      if (uploadResult.succeeded.length > 0) {
        onUploaded(uploadResult.succeeded);
      }

      if (uploadResult.failed.length === 0) {
        toast.success(`已成功上传 ${uploadResult.succeeded.length} 个文件`);
        onCancel();
        return;
      }

      const failedIndices = new Set(uploadResult.failed.map((item) => item.index));
      setFiles((current) => current.filter((_, index) => failedIndices.has(index)));
      toast.warning({
        title: "部分文件上传失败",
        description: `${uploadResult.succeeded.length} 个成功，${uploadResult.failed.length} 个失败。`,
      });
    } catch (uploadError) {
      setError(getErrorMessage(uploadError, "文件上传失败"));
    } finally {
      setUploading(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) onCancel();
      }}
      closeOnEscape={!uploading}
      closeOnOverlayClick={!uploading}
    >
      <DialogOverlay />
      <DialogContent className="max-h-[calc(100vh-48px)] max-w-[520px]">
        <DialogHeader>
          <DialogTitle>上传文件</DialogTitle>
          <DialogClose disabled={uploading} aria-label="关闭上传弹窗">
            <X className="h-4 w-4" aria-hidden />
          </DialogClose>
        </DialogHeader>

        <form onSubmit={(e) => void handleSubmit(e)}>
          <DialogBody className="max-h-none overflow-visible px-card py-space-4">
            <div className="space-y-4">
            <div className="space-y-1.5">
              <label className="block text-sm font-medium text-text-primary">
                文件 <span className="ml-1 text-error">*</span>
              </label>
              <div className="flex items-center gap-3">
                <input
                  ref={fileInputRef}
                  type="file"
                  multiple
                  className="hidden"
                  onChange={handleFileChange}
                />
                <Button
                  type="button"
                  variant="secondary"
                  disabled={uploading}
                  onClick={() => fileInputRef.current?.click()}
                >
                  <Upload className="h-4 w-4" aria-hidden />
                  选择文件
                </Button>
                <span className="min-w-0 flex-1 truncate text-sm text-text-secondary">
                  {files.length === 0
                    ? "未选择文件"
                    : `已选择 ${files.length} 个文件：${files.map((file) => file.name).join("、")}`}
                </span>
                {files.length > 0 && (
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="h-7 w-7 shrink-0"
                    disabled={uploading}
                    onClick={() => {
                      setFiles([]);
                      setResult(null);
                      if (fileInputRef.current) fileInputRef.current.value = "";
                    }}
                    aria-label="清除文件"
                  >
                    <X className="h-3.5 w-3.5" aria-hidden />
                  </Button>
                )}
              </div>
            </div>

            <div className="space-y-1.5">
              <label
                htmlFor="upload-business-module"
                className="block text-sm font-medium text-text-primary"
              >
                业务模块
              </label>
              {businessModuleOptions.length > 0 ? (
                <Select
                  id="upload-business-module"
                  value={businessModule}
                  onChange={(event) => setBusinessModule(event.target.value)}
                  disabled={uploading}
                >
                  <option value="">请选择业务模块</option>
                  {businessModuleOptions.map((item) => (
                    <option key={item.value} value={item.value}>
                      {item.label}
                    </option>
                  ))}
                </Select>
              ) : (
                <Input
                  id="upload-business-module"
                  value={businessModule}
                  onChange={(event) => setBusinessModule(event.target.value)}
                  disabled={uploading}
                  placeholder="例如：user、system-config"
                />
              )}
              <p className="min-h-[18px] text-xs text-text-tertiary">
                可选，用于标识文件所属业务
              </p>
            </div>

            <div className="space-y-1.5">
              <label
                htmlFor="upload-remark"
                className="block text-sm font-medium text-text-primary"
              >
                备注
              </label>
              <Textarea
                id="upload-remark"
                value={remark}
                onChange={(event) => setRemark(event.target.value)}
                disabled={uploading}
                placeholder="补充说明"
                rows={3}
              />
            </div>

            {error && (
              <p className="text-sm text-error">{error}</p>
            )}
            {result && (
              <div className="space-y-2 rounded-control border border-border bg-neutral-background p-space-3 text-sm">
                <p className="font-medium text-text-primary">
                  本次上传：成功 {result.succeeded.length} 个，失败 {result.failed.length} 个
                </p>
                {result.failed.length > 0 && (
                  <ul className="space-y-1 text-error">
                    {result.failed.map((item, index) => (
                      <li key={`${item.fileName}-${index}`}>
                        {item.fileName}：{item.message}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
            </div>
          </DialogBody>

          <DialogFooter className="mt-5">
            <Button
              type="button"
              variant="secondary"
              disabled={uploading}
              onClick={onCancel}
            >
              取消
            </Button>
            <Button
              type="submit"
              variant="primary"
              disabled={uploading || files.length === 0}
            >
              {uploading ? "上传中..." : result?.failed.length ? "重试失败项" : "上传"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
