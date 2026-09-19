import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

type DetailItemProps = {
  label: string;
  value?: ReactNode;
  className?: string;
};

export function DetailItem({ label, value, className }: DetailItemProps) {
  const isEmpty = value === undefined || value === null || value === "";

  return (
    <div className={cn("min-w-0", className)}>
      <div className="text-body-secondary text-text-tertiary">{label}</div>
      <div className="mt-space-1 break-words text-sm text-text-primary">
        {isEmpty ? "-" : value}
      </div>
    </div>
  );
}
