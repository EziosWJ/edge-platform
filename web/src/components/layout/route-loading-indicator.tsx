import { useNavigation } from "react-router-dom";

type RouteLoadingIndicatorProps = {
  visible?: boolean;
};

export function RouteLoadingIndicator({ visible = false }: RouteLoadingIndicatorProps) {
  const navigation = useNavigation();

  if (!visible && navigation.state !== "loading") return null;

  return (
    <div
      className="pointer-events-none fixed inset-x-0 top-0 z-[60] h-1 bg-primary/20"
      role="status"
      aria-live="polite"
      aria-label="正在切换页面"
    >
      <div className="h-full w-1/3 animate-pulse bg-primary shadow-[0_0_10px_var(--color-primary)]" />
      <span className="sr-only">正在切换页面…</span>
    </div>
  );
}
