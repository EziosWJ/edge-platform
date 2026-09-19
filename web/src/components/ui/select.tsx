import { forwardRef, type SelectHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

type SelectProps = SelectHTMLAttributes<HTMLSelectElement>;

export const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({ className, ...props }, ref) => {
    return (
      <select
        ref={ref}
        className={cn(
          "h-control-md w-full rounded-control border border-border bg-surface px-space-3 text-sm text-text-primary outline-none transition-colors hover:border-border focus:border-primary disabled:cursor-not-allowed disabled:bg-neutral-background disabled:text-text-tertiary",
          className,
        )}
        {...props}
      />
    );
  },
);

Select.displayName = "Select";
