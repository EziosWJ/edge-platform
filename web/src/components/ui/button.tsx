import type { ButtonHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
type ButtonSize = "sm" | "md" | "lg" | "icon";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
};

const variants: Record<ButtonVariant, string> = {
  primary:
    "border-primary bg-primary text-white hover:bg-primary-hover active:bg-primary-hover disabled:border-primary/50 disabled:bg-primary/50",
  secondary:
    "border-border bg-surface text-text-primary hover:bg-neutral-background active:bg-neutral-background disabled:text-text-tertiary",
  ghost:
    "border-transparent bg-transparent text-text-secondary hover:bg-neutral-background hover:text-text-primary active:bg-neutral-background disabled:text-text-tertiary",
  danger:
    "border-error bg-error !text-white hover:bg-error hover:!text-white active:bg-error active:!text-white disabled:border-error/50 disabled:bg-error/50 disabled:!text-white",
};

const sizes: Record<ButtonSize, string> = {
  sm: "h-control-sm gap-1.5 px-space-3 text-body-secondary",
  md: "h-control-md gap-space-2 px-space-4 text-sm",
  lg: "h-control-lg gap-space-2 px-space-5 text-sm",
  icon: "h-control-md w-9 p-0",
};

export function Button({
  className,
  variant = "secondary",
  size = "md",
  type = "button",
  ...props
}: ButtonProps) {
  return (
    <button
      type={type}
      className={cn(
        "inline-flex shrink-0 items-center justify-center whitespace-nowrap rounded-control border font-medium transition-colors disabled:pointer-events-none",
        variants[variant],
        sizes[size],
        className,
      )}
      {...props}
    />
  );
}
