import { forwardRef, type InputHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

type InputProps = InputHTMLAttributes<HTMLInputElement>;

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, ...props }, ref) => {
    return (
      <input
        ref={ref}
        className={cn(
          "h-control-md w-full rounded-control border border-border bg-surface px-space-3 text-sm text-text-primary outline-none transition-colors placeholder:text-text-tertiary hover:border-border focus:border-primary disabled:cursor-not-allowed disabled:bg-neutral-background disabled:text-text-tertiary",
          className,
        )}
        {...props}
      />
    );
  },
);

Input.displayName = "Input";
