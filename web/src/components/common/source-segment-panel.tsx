import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import type { SourceSegmentVO } from "@/types/material";

type SourceSegmentPanelProps = {
  segments: SourceSegmentVO[];
  title?: string;
};

export function SourceSegmentPanel({
  segments,
  title = "来源片段",
}: SourceSegmentPanelProps) {
  const [expandedSegments, setExpandedSegments] = useState<Set<number>>(
    new Set(),
  );

  const toggleSegment = (index: number) => {
    setExpandedSegments((prev) => {
      const next = new Set(prev);
      if (next.has(index)) {
        next.delete(index);
      } else {
        next.add(index);
      }
      return next;
    });
  };

  return (
    <div className="p-space-4">
      <h4 className="mb-space-3 text-xs font-medium text-text-tertiary">{title}</h4>
      <div className="space-y-2">
        {segments.length > 0 ? (
          segments.map((segment, index) => (
            <div
              key={`${segment.materialId}-${segment.segmentIndex}-${index}`}
              className="rounded-control border border-border"
            >
              <button
                className="flex w-full items-start gap-space-2 px-space-3 py-space-2 text-left"
                onClick={() => toggleSegment(index)}
              >
                {expandedSegments.has(index) ? (
                  <ChevronDown className="mt-0.5 h-4 w-4 flex-shrink-0 text-text-tertiary" />
                ) : (
                  <ChevronRight className="mt-0.5 h-4 w-4 flex-shrink-0 text-text-tertiary" />
                )}
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-space-2">
                    <span className="truncate text-sm font-medium text-text-primary">
                      {segment.materialTitle}
                    </span>
                  </div>
                  <div className="mt-space-1 flex items-center gap-space-2 text-xs text-text-tertiary">
                    <span>片段 {segment.segmentIndex}</span>
                    <span>·</span>
                    <span>相关度 {(segment.score * 100).toFixed(0)}%</span>
                  </div>
                </div>
              </button>
              {expandedSegments.has(index) && (
                <div className="border-t border-border px-space-3 py-space-2">
                  <p className="text-xs leading-relaxed text-text-secondary">
                    {segment.text}
                  </p>
                </div>
              )}
            </div>
          ))
        ) : (
          <p className="text-xs text-text-tertiary">暂无来源片段</p>
        )}
      </div>
    </div>
  );
}
