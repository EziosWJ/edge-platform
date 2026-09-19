import { CheckCircle2, Upload, X } from "lucide-react";
import { useRef, useState, type ChangeEvent } from "react";
import { uploadFile } from "@/api/file";
import { Button } from "@/components/ui/button";
import { isApiError } from "@/lib/api-error";
import { cn } from "@/lib/utils";
import type { FileRecord, FileUploadOptions } from "@/types/file";

type FileUploadProps = FileUploadOptions & {
  accept?: string;
  disabled?: boolean;
  buttonText?: string;
  helperText?: string;
  className?: string;
  onUploaded?: (file: FileRecord) => void;
  onAccessUrlChange?: (accessUrl: string, file: FileRecord) => void;
  onCleared?: () => void;
};

function getErrorMessage(error: unknown) {
  if (isApiError(error)) return error.message;
  if (error instanceof Error) return error.message;
  return "文件上传失败";
}

export function FileUpload({
  accept,
  disabled = false,
  buttonText = "选择文件",
  helperText,
  businessModule,
  remark,
  className,
  onUploaded,
  onAccessUrlChange,
  onCleared,
}: FileUploadProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [selectedName, setSelectedName] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  const [uploadedFile, setUploadedFile] = useState<FileRecord | null>(null);

  const handleSelect = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    setSelectedName(file.name);
    setErrorMessage("");
    setUploadedFile(null);
    onCleared?.();
    setUploading(true);

    try {
      const uploadedFile = await uploadFile(file, {
        businessModule,
        remark,
      });
      setUploadedFile(uploadedFile);
      onUploaded?.(uploadedFile);
      onAccessUrlChange?.(uploadedFile.accessUrl, uploadedFile);
    } catch (error) {
      setErrorMessage(getErrorMessage(error));
    } finally {
      setUploading(false);
      event.target.value = "";
    }
  };

  const handleClear = () => {
    setSelectedName("");
    setErrorMessage("");
    setUploadedFile(null);
    onCleared?.();
    if (inputRef.current) {
      inputRef.current.value = "";
    }
  };

  return (
    <div className={cn("space-y-2", className)}>
      <div className="flex flex-wrap items-center gap-space-2">
        <input
          ref={inputRef}
          type="file"
          accept={accept}
          disabled={disabled || uploading}
          className="sr-only"
          onChange={handleSelect}
        />
        <Button
          type="button"
          variant="secondary"
          disabled={disabled || uploading}
          className={cn(uploading && "cursor-wait")}
          onClick={() => inputRef.current?.click()}
        >
          <Upload className="h-4 w-4" aria-hidden />
          {uploading ? "上传中..." : buttonText}
        </Button>
        {selectedName && (
          <div className="flex min-w-0 items-center gap-space-2 rounded-control border border-border bg-surface px-space-3 py-1.5 text-sm text-text-secondary">
            <span className="max-w-[240px] truncate">{selectedName}</span>
            <button
              type="button"
              className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-tight text-text-tertiary hover:bg-neutral-background hover:text-text-primary"
              disabled={uploading}
              onClick={handleClear}
              aria-label="清除已选文件"
            >
              <X className="h-3.5 w-3.5" aria-hidden />
            </button>
          </div>
        )}
      </div>
      {helperText && (
        <p className="text-xs leading-5 text-text-tertiary">{helperText}</p>
      )}
      {uploadedFile && (
        <div className="flex items-start gap-space-2 rounded-control border border-success-border bg-success-background px-space-3 py-space-2 text-xs text-success" role="status">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
          <div className="min-w-0">
            <p className="font-medium">上传成功</p>
            <p className="mt-0.5 truncate">文件 ID {uploadedFile.id} · {uploadedFile.fileSize} B</p>
          </div>
        </div>
      )}
      {errorMessage && (
        <p className="text-xs leading-5 text-error" role="alert">{errorMessage}</p>
      )}
    </div>
  );
}
