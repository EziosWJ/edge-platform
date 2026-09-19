import { cn } from "@/lib/utils";

type StatusTone = "success" | "warning" | "error" | "info" | "neutral";

type StatusTagProps = {
  children: string;
  tone?: StatusTone;
};

const tones: Record<StatusTone, string> = {
  success: "border-success-border bg-success-background text-success",
  warning: "border-warning-border bg-warning-background text-warning",
  error: "border-error-border bg-error-background text-error",
  info: "border-info-border bg-info-background text-info",
  neutral: "border-neutral-border bg-neutral-background text-text-secondary",
};

export function StatusTag({ children, tone = "neutral" }: StatusTagProps) {
  return (
    <span
      className={cn(
        "inline-flex h-6 items-center rounded-tag border px-space-2 text-xs font-medium",
        tones[tone],
      )}
    >
      {children}
    </span>
  );
}
