import { useCallback, useEffect, useRef, useState } from "react";
import type { ApiPageResult } from "@/types/api";

type UseListPageOptions<TFilters, TRecord, TQuery extends Record<string, unknown> = Record<string, unknown>> = {
  fetch: (query: TQuery) => Promise<ApiPageResult<TRecord>>;
  defaultFilters: TFilters;
  toQuery: (filters: TFilters, page: number, pageSize: number) => TQuery;
  defaultPageSize?: number;
  onError?: (error: unknown) => void;
};

type UseListPageReturn<TFilters, TRecord> = {
  data: TRecord[];
  total: number;
  loading: boolean;
  error: string;
  page: number;
  pageSize: number;
  setPage: (page: number) => void;
  setPageSize: (size: number) => void;
  filters: TFilters;
  setFilter: <K extends keyof TFilters>(key: K, value: TFilters[K]) => void;
  setFilters: (updater: (prev: TFilters) => TFilters) => void;
  submitFilters: () => void;
  resetFilters: () => void;
  reload: () => void;
};

export function useListPage<TFilters, TRecord, TQuery extends Record<string, unknown> = Record<string, unknown>>(
  options: UseListPageOptions<TFilters, TRecord, TQuery>,
): UseListPageReturn<TFilters, TRecord> {
  const { fetch, defaultFilters, toQuery, defaultPageSize = 10, onError } = options;

  const [data, setData] = useState<TRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(defaultPageSize);
  const [queryVersion, setQueryVersion] = useState(0);

  const [filters, setFiltersState] = useState<TFilters>(defaultFilters);
  const [appliedFilters, setAppliedFilters] = useState<TFilters>(defaultFilters);

  // 用 ref 缓存调用方传入的函数，避免内联函数引用变化导致循环请求
  const fetchRef = useRef(fetch);
  fetchRef.current = fetch;
  const toQueryRef = useRef(toQuery);
  toQueryRef.current = toQuery;
  const onErrorRef = useRef(onError);
  onErrorRef.current = onError;

  const loadData = useCallback(async () => {
    setLoading(true);
    setError("");

    try {
      const result = await fetchRef.current(toQueryRef.current(appliedFilters, page, pageSize));
      setData(result.records);
      setTotal(result.total);
    } catch (loadError) {
      setData([]);
      setTotal(0);
      if (onErrorRef.current) {
        onErrorRef.current(loadError);
      }
      setError(
        loadError instanceof Error ? loadError.message : "加载失败",
      );
    } finally {
      setLoading(false);
    }
  }, [appliedFilters, page, pageSize]);

  useEffect(() => {
    void loadData();
  }, [loadData, queryVersion]);

  const setPageSizeAndReset = useCallback((size: number) => {
    setPageSize(size);
    setPage(1);
  }, []);

  const setFilter = useCallback(
    <K extends keyof TFilters>(key: K, value: TFilters[K]) => {
      setFiltersState((prev) => ({ ...prev, [key]: value }));
    },
    [],
  );

  const setFilters = useCallback(
    (updater: (prev: TFilters) => TFilters) => {
      setFiltersState(updater);
    },
    [],
  );

  const submitFilters = useCallback(() => {
    setPage(1);
    setAppliedFilters(filters);
    setQueryVersion((version) => version + 1);
  }, [filters]);

  const resetFilters = useCallback(() => {
    setFiltersState(defaultFilters);
    setAppliedFilters(defaultFilters);
    setPage(1);
  }, [defaultFilters]);

  const reload = useCallback(() => {
    void loadData();
  }, [loadData]);

  return {
    data,
    total,
    loading,
    error,
    page,
    pageSize,
    setPage,
    setPageSize: setPageSizeAndReset,
    filters,
    setFilter,
    setFilters,
    submitFilters,
    resetFilters,
    reload,
  };
}
