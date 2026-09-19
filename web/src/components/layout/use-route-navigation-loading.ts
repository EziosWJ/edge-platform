import { useEffect, useRef, useState, type MouseEvent } from "react";
import { useLocation, useNavigation } from "react-router-dom";

const MINIMUM_LOADING_MS = 180;

export function useRouteNavigationLoading() {
  const [navigationIntent, setNavigationIntent] = useState(false);
  const navigation = useNavigation();
  const location = useLocation();
  const navigationStartedAt = useRef<number | null>(null);
  const clearNavigationTimer = useRef<number | null>(null);

  const beginNavigation = () => {
    if (clearNavigationTimer.current !== null) {
      window.clearTimeout(clearNavigationTimer.current);
      clearNavigationTimer.current = null;
    }
    navigationStartedAt.current ??= Date.now();
    setNavigationIntent(true);
  };

  useEffect(() => {
    if (navigation.state === "loading" || !navigationIntent) return;

    const elapsed = Date.now() - (navigationStartedAt.current ?? Date.now());
    const remaining = Math.max(0, MINIMUM_LOADING_MS - elapsed);
    clearNavigationTimer.current = window.setTimeout(() => {
      navigationStartedAt.current = null;
      clearNavigationTimer.current = null;
      setNavigationIntent(false);
    }, remaining);

    return () => {
      if (clearNavigationTimer.current !== null) {
        window.clearTimeout(clearNavigationTimer.current);
        clearNavigationTimer.current = null;
      }
    };
  }, [location.key, navigation.state, navigationIntent]);

  useEffect(() => () => {
    if (clearNavigationTimer.current !== null) window.clearTimeout(clearNavigationTimer.current);
  }, []);

  const handleNavigationClickCapture = (event: MouseEvent<HTMLDivElement>) => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    const target = event.target;
    if (!(target instanceof Element)) return;
    const link = target.closest("a");
    if (!link || link.target && link.target !== "_self" || link.hasAttribute("download")) return;
    const href = link.getAttribute("href");
    if (!href || href.startsWith("#")) return;

    const destination = new URL(href, window.location.href);
    const current = `${location.pathname}${location.search}${location.hash}`;
    if (destination.origin !== window.location.origin || `${destination.pathname}${destination.search}${destination.hash}` === current) return;
    beginNavigation();
  };

  return { navigationIntent, handleNavigationClickCapture };
}
