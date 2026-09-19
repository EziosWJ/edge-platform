import { useCallback, useEffect, useMemo, useState } from "react";
import { getDictItems } from "@/api/system";
import { toast } from "@/components/common/toast-store";
import type { DictSelectOption } from "@/constants/dicts";
import type { DictOption } from "@/types";

type DictValue = string | number;

type UseDictOptionsOptions<T extends DictValue> = {
  fallback?: readonly DictSelectOption<T>[];
  allowedValues?: readonly T[];
  enabled?: boolean;
  valueType?: "string" | "number";
  showErrorToast?: boolean;
  errorTitle?: string;
};

type UseDictOptionsResult<T extends DictValue> = {
  options: DictSelectOption<T>[];
  loading: boolean;
  error: string;
  reload: () => void;
};

function toFallbackOptions<T extends DictValue>(
  fallback?: readonly DictSelectOption<T>[],
) {
  return fallback ? [...fallback] : [];
}

function parseDictValue<T extends DictValue>(
  value: string,
  valueType: UseDictOptionsOptions<T>["valueType"],
) {
  if (valueType !== "number") return value as T;

  const parsed = Number(value);
  return (Number.isNaN(parsed) ? value : parsed) as T;
}

function normalizeDictOptions<T extends DictValue>(
  items: DictOption[],
  options: UseDictOptionsOptions<T>,
) {
  const allowedValueSet = options.allowedValues
    ? new Set<DictValue>(options.allowedValues)
    : null;

  const normalized = items
    .map((item) => ({
      label: item.label,
      value: parseDictValue<T>(item.value, options.valueType),
      sortOrder: item.sortOrder,
    }))
    .filter((item) => !allowedValueSet || allowedValueSet.has(item.value))
    .sort((prev, next) => prev.sortOrder - next.sortOrder)
    .map(({ label, value }) => ({ label, value }));

  return normalized.length > 0
    ? normalized
    : toFallbackOptions(options.fallback);
}

function getErrorMessage(error: unknown) {
  if (error instanceof Error) return error.message;
  return "字典选项加载失败";
}

export function useDictOptions<T extends DictValue = string>(
  dictCode: string,
  options: UseDictOptionsOptions<T> = {},
): UseDictOptionsResult<T> {
  const {
    allowedValues,
    enabled = true,
    errorTitle = "字典选项加载失败",
    fallback,
    showErrorToast = false,
    valueType,
  } = options;
  const [dictOptions, setDictOptions] = useState<DictSelectOption<T>[]>(() =>
    toFallbackOptions(fallback),
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [reloadKey, setReloadKey] = useState(0);

  const fallbackOptions = useMemo(() => toFallbackOptions(fallback), [fallback]);

  const reload = useCallback(() => {
    setReloadKey((current) => current + 1);
  }, []);

  useEffect(() => {
    if (!enabled || !dictCode.trim()) {
      setDictOptions(fallbackOptions);
      setLoading(false);
      setError("");
      return;
    }

    let ignored = false;
    setLoading(true);
    setError("");

    getDictItems(dictCode, reloadKey > 0)
      .then((items) => {
        if (ignored) return;
        setDictOptions(
          normalizeDictOptions(items, {
            allowedValues,
            fallback,
            valueType,
          }),
        );
      })
      .catch((loadError) => {
        if (ignored) return;

        const message = getErrorMessage(loadError);
        setDictOptions(fallbackOptions);
        setError(message);

        if (showErrorToast) {
          toast.error({
            title: errorTitle,
            description: message,
          });
        }
      })
      .finally(() => {
        if (!ignored) {
          setLoading(false);
        }
      });

    return () => {
      ignored = true;
    };
  }, [
    dictCode,
    enabled,
    errorTitle,
    fallbackOptions,
    allowedValues,
    fallback,
    reloadKey,
    showErrorToast,
    valueType,
  ]);

  return {
    options: dictOptions,
    loading,
    error,
    reload,
  };
}
