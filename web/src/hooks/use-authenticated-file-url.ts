import { useEffect, useState } from "react";
import { getErrorMessage } from "@/lib/api-error";
import { buildApiUrl, http } from "@/lib/http";

type AuthenticatedFileUrl = {
  url: string;
  loading: boolean;
  error: string;
};

function isTrustedFileViewPath(path: string) {
  if (/^(data:|blob:)/i.test(path)) return false;

  try {
    const resolved = new URL(buildApiUrl(path), window.location.origin);
    const apiOrigin = new URL(
      buildApiUrl("/"),
      window.location.origin,
    ).origin;
    return (
      resolved.origin === apiOrigin &&
      /^\/api\/system\/file\/\d+\/view$/.test(resolved.pathname)
    );
  } catch {
    return false;
  }
}

export function useAuthenticatedFileUrl(
  path: string | null | undefined,
): AuthenticatedFileUrl {
  const [url, setUrl] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!path) {
      setUrl("");
      setLoading(false);
      setError("");
      return;
    }

    const controller = new AbortController();
    let objectUrl = "";

    if (!isTrustedFileViewPath(path)) {
      setUrl(/^(https?:\/\/|data:|blob:)/i.test(path) ? path : buildApiUrl(path));
      setLoading(false);
      setError("");
      return () => controller.abort();
    }

    setUrl("");
    setLoading(true);
    setError("");

    http
      .blob(path, { signal: controller.signal })
      .then((blob) => {
        if (controller.signal.aborted) return;
        objectUrl = URL.createObjectURL(blob);
        setUrl(objectUrl);
      })
      .catch((loadError) => {
        if (controller.signal.aborted) return;
        setError(getErrorMessage(loadError, "文件加载失败"));
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      });

    return () => {
      controller.abort();
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [path]);

  return { url, loading, error };
}
