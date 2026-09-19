import { FileSearch } from "lucide-react";
import { Button } from "@/components/ui/button";

type EmptyStateProps = {
  title: string;
  description?: string;
  actionText?: string;
  onAction?: () => void;
};

export function EmptyState({
  title,
  description,
  actionText,
  onAction,
}: EmptyStateProps) {
  return (
    <div className="flex min-h-64 flex-col items-center justify-center rounded-admin border border-dashed border-border bg-neutral-background px-space-6 py-10 text-center">
      <FileSearch className="h-10 w-10 text-text-tertiary" aria-hidden />
      <h2 className="mt-space-4 text-base font-semibold text-text-primary">
        {title}
      </h2>
      {description && (
        <p className="mt-space-1 max-w-md text-sm text-text-tertiary">
          {description}
        </p>
      )}
      {actionText && onAction && (
      <Button className="mt-space-4" variant="primary" onClick={onAction}>
          {actionText}
        </Button>
      )}
    </div>
  );
}
