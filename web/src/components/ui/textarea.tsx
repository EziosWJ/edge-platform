import { forwardRef, type TextareaHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, ...props }, ref) => {
    return (
      <textarea
        ref={ref}
        className={cn(
          "min-h-control-textarea w-full rounded-control border border-border bg-surface px-space-3 py-space-2 text-sm text-text-primary outline-none transition-colors placeholder:text-text-tertiary hover:border-border focus:border-primary disabled:cursor-not-allowed disabled:bg-neutral-background disabled:text-text-tertiary",
          className,
        )}
        {...props}
      />
    );
  },
);

Textarea.displayName = "Textarea";
