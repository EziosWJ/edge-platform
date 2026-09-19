import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
  type DialogContentProps,
  type DialogProps,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

export type DrawerProps = DialogProps;

export function Drawer({ lockScroll = true, ...props }: DrawerProps) {
  return <Dialog lockScroll={lockScroll} {...props} />;
}

export const DrawerOverlay = DialogOverlay;
export const DrawerHeader = DialogHeader;
export const DrawerTitle = DialogTitle;
export const DrawerDescription = DialogDescription;
export const DrawerBody = DialogBody;
export const DrawerFooter = DialogFooter;
export const DrawerClose = DialogClose;

export type DrawerSide = "left" | "right";
export type DrawerSize = "sm" | "md" | "lg";

export type DrawerContentProps = DialogContentProps & {
  side?: DrawerSide;
  size?: DrawerSize;
};

const sizeClasses: Record<DrawerSize, string> = {
  sm: "max-w-modal-sm",
  md: "max-w-modal-md",
  lg: "max-w-modal-lg",
};

export function DrawerContent({
  side = "right",
  size = "md",
  className,
  ...props
}: DrawerContentProps) {
  return (
    <DialogContent
      {...props}
      className={cn(
        "!top-0 !h-full !max-h-none !translate-x-0 !translate-y-0 !rounded-none",
        side === "left"
          ? "!left-0 !right-auto !border-r-0"
          : "!left-auto !right-0 !border-l-0",
        sizeClasses[size],
        className,
      )}
    />
  );
}
