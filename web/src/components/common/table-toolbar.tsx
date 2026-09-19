import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

type TableToolbarProps = {
  title?: string;
  description?: string;
  actions?: ReactNode;
  className?: string;
};

export function TableToolbar({
  title,
  description,
  actions,
  className,
}: TableToolbarProps) {
  return (
    <div
      className={cn(
        "flex flex-col gap-space-3 border-b border-border px-card py-space-4 md:flex-row md:items-start md:justify-between",
        className,
      )}
    >
      <div>
        {title && <h2 className="text-card-title font-semibold">{title}</h2>}
        {description && (
          <p className="mt-space-1 text-body-secondary text-text-tertiary">{description}</p>
        )}
      </div>
      {actions && <div className="flex items-center gap-space-2">{actions}</div>}
    </div>
  );
}
