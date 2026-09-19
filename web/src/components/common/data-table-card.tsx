import type { PropsWithChildren, ReactNode } from "react";
import { cn } from "@/lib/utils";

type DataTableCardProps = PropsWithChildren<{
  toolbar?: ReactNode;
  pagination?: ReactNode;
  className?: string;
}>;

export function DataTableCard({
  toolbar,
  pagination,
  className,
  children,
}: DataTableCardProps) {
  return (
    <section
      className={cn(
        "rounded-admin border border-border bg-surface shadow-admin",
        className,
      )}
    >
      {toolbar}
      {children}
      {pagination}
    </section>
  );
}
