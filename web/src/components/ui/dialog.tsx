import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type ComponentPropsWithoutRef,
  type HTMLAttributes,
  type MouseEvent,
  type MouseEventHandler,
  type MutableRefObject,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  closeOnEscape?: boolean;
  closeOnOverlayClick?: boolean;
  trapFocus?: boolean;
  restoreFocus?: boolean;
  lockScroll?: boolean;
  onEscapeKeyDown?: (event: KeyboardEvent) => void;
  children: ReactNode;
};

type DialogContextValue = {
  titleId: string;
  descriptionId: string;
  hasDescription: boolean;
  registerDescription: (id: string | null) => void;
  contentRef: MutableRefObject<HTMLElement | null>;
  closeOnEscape: boolean;
  closeOnOverlayClick: boolean;
  onOpenChange: (open: boolean) => void;
};

const DialogContext = createContext<DialogContextValue | null>(null);

function useDialogContext() {
  const context = useContext(DialogContext);
  if (!context) {
    throw new Error("Dialog primitives must be used inside Dialog.");
  }
  return context;
}

const focusableSelector =
  'button:not([disabled]):not([tabindex="-1"]), [href], input:not([disabled]):not([tabindex="-1"]), select:not([disabled]):not([tabindex="-1"]), textarea:not([disabled]):not([tabindex="-1"]), [tabindex]:not([tabindex="-1"])';

export function Dialog({
  open,
  onOpenChange,
  closeOnEscape = true,
  closeOnOverlayClick = true,
  trapFocus = false,
  restoreFocus = false,
  lockScroll = false,
  onEscapeKeyDown,
  children,
}: DialogProps) {
  const titleId = useId();
  const generatedDescriptionId = useId();
  const contentRef = useRef<HTMLElement | null>(null);
  const [hasDescription, setHasDescription] = useState(false);
  const [descriptionId, setDescriptionId] = useState(generatedDescriptionId);
  const optionsRef = useRef({
    closeOnEscape,
    closeOnOverlayClick,
    trapFocus,
    lockScroll,
    onEscapeKeyDown,
  });
  const onOpenChangeRef = useRef(onOpenChange);
  const registerDescription = useCallback(
    (id: string | null) => {
      setHasDescription(id !== null);
      setDescriptionId(id ?? generatedDescriptionId);
    },
    [generatedDescriptionId],
  );

  optionsRef.current = {
    closeOnEscape,
    closeOnOverlayClick,
    trapFocus,
    lockScroll,
    onEscapeKeyDown,
  };
  onOpenChangeRef.current = onOpenChange;

  useEffect(() => {
    if (!open || typeof document === "undefined") return;

    const previousFocus = document.activeElement;
    const previousOverflow = document.body.style.overflow;
    const shouldRestoreFocus = restoreFocus;
    const shouldLockScroll = optionsRef.current.lockScroll;

    if (shouldLockScroll) {
      document.body.style.overflow = "hidden";
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        optionsRef.current.onEscapeKeyDown?.(event);
        if (!event.defaultPrevented && optionsRef.current.closeOnEscape) {
          event.preventDefault();
          onOpenChangeRef.current(false);
        }
        return;
      }

      if (event.key !== "Tab" || !optionsRef.current.trapFocus) return;

      const content = contentRef.current;
      if (!content) return;

      const focusable = Array.from(
        content.querySelectorAll<HTMLElement>(focusableSelector),
      );
      if (focusable.length === 0) return;

      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      if (shouldLockScroll) {
        document.body.style.overflow = previousOverflow;
      }
      if (shouldRestoreFocus && previousFocus instanceof HTMLElement) {
        previousFocus.focus();
      }
    };
  }, [open, restoreFocus]);

  if (!open || typeof document === "undefined") return null;

  return createPortal(
    <DialogContext.Provider
      value={{
        titleId,
        descriptionId,
        hasDescription,
        registerDescription,
        contentRef,
        closeOnEscape,
        closeOnOverlayClick,
        onOpenChange,
      }}
    >
      <div className="fixed inset-0 z-50" role="presentation">
        {children}
      </div>
    </DialogContext.Provider>,
    document.body,
  );
}

export type DialogOverlayProps = Omit<
  HTMLAttributes<HTMLDivElement>,
  "children"
>;

export function DialogOverlay({
  className,
  onMouseDown,
  ...props
}: DialogOverlayProps) {
  const { closeOnOverlayClick, onOpenChange } = useDialogContext();

  const handleMouseDown: MouseEventHandler<HTMLDivElement> = (event) => {
    onMouseDown?.(event);
    if (
      !event.defaultPrevented &&
      closeOnOverlayClick &&
      event.target === event.currentTarget
    ) {
      onOpenChange(false);
    }
  };

  return (
    <div
      {...props}
      aria-hidden="true"
      className={cn("fixed inset-0 z-0 bg-overlay-background", className)}
      onMouseDown={handleMouseDown}
    />
  );
}

export type DialogContentProps = HTMLAttributes<HTMLElement>;

export function DialogContent({
  className,
  children,
  onKeyDown,
  "aria-labelledby": labelledBy,
  "aria-describedby": describedBy,
  ...props
}: DialogContentProps) {
  const { titleId, descriptionId, hasDescription, contentRef } =
    useDialogContext();

  return (
    <section
      {...props}
      ref={contentRef}
      role="dialog"
      aria-modal="true"
      aria-labelledby={labelledBy ?? titleId}
      aria-describedby={
        describedBy ?? (hasDescription ? descriptionId : undefined)
      }
      className={cn(
        "fixed left-1/2 top-1/2 z-10 max-h-[calc(100vh-48px)] w-[calc(100%-32px)] -translate-x-1/2 -translate-y-1/2 overflow-hidden rounded-admin border border-border bg-surface shadow-admin",
        className,
      )}
      onKeyDown={onKeyDown}
    >
      {children}
    </section>
  );
}

export type DialogHeaderProps = HTMLAttributes<HTMLElement>;

export function DialogHeader({ className, ...props }: DialogHeaderProps) {
  return (
    <header
      {...props}
      className={cn(
        "flex items-start justify-between gap-space-4 border-b border-border px-card py-space-4",
        className,
      )}
    />
  );
}

export type DialogTitleProps = Omit<
  HTMLAttributes<HTMLHeadingElement>,
  "id"
>;

export function DialogTitle({ className, ...props }: DialogTitleProps) {
  const { titleId } = useDialogContext();
  return <h2 {...props} id={titleId} className={cn("text-base font-semibold text-text-primary", className)} />;
}

export type DialogDescriptionProps = HTMLAttributes<HTMLDivElement>;

export function DialogDescription({
  className,
  id,
  ...props
}: DialogDescriptionProps) {
  const { descriptionId, registerDescription } = useDialogContext();
  const resolvedId = id ?? descriptionId;

  useEffect(() => {
    registerDescription(resolvedId);
    return () => registerDescription(null);
  }, [registerDescription, resolvedId]);

  return (
    <div
      {...props}
      id={resolvedId}
      className={cn("mt-space-1 text-body-secondary text-text-tertiary", className)}
    />
  );
}

export type DialogBodyProps = HTMLAttributes<HTMLDivElement>;

export function DialogBody({ className, ...props }: DialogBodyProps) {
  return (
    <div
      {...props}
      className={cn("min-h-0 overflow-y-auto px-card py-space-5", className)}
    />
  );
}

export type DialogFooterProps = HTMLAttributes<HTMLElement>;

export function DialogFooter({ className, ...props }: DialogFooterProps) {
  return (
    <footer
      {...props}
      className={cn(
        "flex justify-end gap-space-2 border-t border-border px-card py-space-4",
        className,
      )}
    />
  );
}

export type DialogCloseProps = Omit<
  ComponentPropsWithoutRef<typeof Button>,
  "size" | "variant"
> & {
  onClick?: MouseEventHandler<HTMLButtonElement>;
};

export function DialogClose({
  className,
  onClick,
  disabled,
  ...props
}: DialogCloseProps) {
  const { onOpenChange } = useDialogContext();

  const handleClick = (event: MouseEvent<HTMLButtonElement>) => {
    onClick?.(event);
    if (!event.defaultPrevented && !disabled) {
      onOpenChange(false);
    }
  };

  return (
    <Button
      {...props}
      type="button"
      size="icon"
      variant="ghost"
      disabled={disabled}
      className={cn("h-control-sm w-8 shrink-0", className)}
      onClick={handleClick}
    />
  );
}
