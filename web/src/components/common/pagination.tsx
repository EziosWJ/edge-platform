import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { cn } from "@/lib/utils";

type PaginationProps = {
  page: number;
  pageSize: number;
  total: number;
  pageSizeOptions?: number[];
  disabled?: boolean;
  className?: string;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (pageSize: number) => void;
};

function clampPage(page: number, pageCount: number) {
  return Math.min(Math.max(page, 1), pageCount);
}

export function Pagination({
  page,
  pageSize,
  total,
  pageSizeOptions = [10, 20, 50, 100],
  disabled = false,
  className,
  onPageChange,
  onPageSizeChange,
}: PaginationProps) {
  const safeTotal = Math.max(total, 0);
  const safePageSize = Math.max(pageSize, 1);
  const pageCount = Math.max(Math.ceil(safeTotal / safePageSize), 1);
  const currentPage = clampPage(page, pageCount);
  const start = safeTotal === 0 ? 0 : (currentPage - 1) * safePageSize + 1;
  const end = Math.min(currentPage * safePageSize, safeTotal);
  const canPrevious = currentPage > 1 && !disabled;
  const canNext = currentPage < pageCount && !disabled;

  return (
    <div
      className={cn(
        "flex flex-col gap-space-3 border-t border-border px-card py-space-3 text-sm text-text-secondary md:flex-row md:items-center md:justify-between",
        className,
      )}
    >
      <div className="tabular-nums">
        共 <span className="font-medium text-text-primary">{safeTotal}</span>{" "}
        条
        {safeTotal > 0 && (
          <>
            ，当前{" "}
            <span className="font-medium text-text-primary">
              {start}-{end}
            </span>
          </>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-space-2">
        {onPageSizeChange && (
          <label className="flex items-center gap-space-2">
            <span>每页</span>
            <Select
              className="h-control-sm w-[84px]"
              value={String(safePageSize)}
              disabled={disabled}
              onChange={(event) => onPageSizeChange(Number(event.target.value))}
              aria-label="每页条数"
            >
              {pageSizeOptions.map((option) => (
                <option key={option} value={String(option)}>
                  {option} 条
                </option>
              ))}
            </Select>
          </label>
        )}

        <div className="flex items-center gap-space-2 tabular-nums">
          <Button
            size="sm"
            variant="secondary"
            disabled={!canPrevious}
            onClick={() => onPageChange(currentPage - 1)}
            aria-label="上一页"
          >
            <ChevronLeft className="h-4 w-4" aria-hidden />
            上一页
          </Button>
          <span className="min-w-16 text-center">
            {currentPage} / {pageCount}
          </span>
          <Button
            size="sm"
            variant="secondary"
            disabled={!canNext}
            onClick={() => onPageChange(currentPage + 1)}
            aria-label="下一页"
          >
            下一页
            <ChevronRight className="h-4 w-4" aria-hidden />
          </Button>
        </div>
      </div>
    </div>
  );
}
