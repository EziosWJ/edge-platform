import { ChevronDown, ChevronRight, X } from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export type TreeSelectNode = {
  id: string | number;
  label: string;
  disabled?: boolean;
  children?: TreeSelectNode[];
};

type TreeSelectProps = {
  value?: string | number | null;
  nodes: TreeSelectNode[];
  placeholder?: string;
  disabled?: boolean;
  clearable?: boolean;
  className?: string;
  onChange: (value: string | number | null, node: TreeSelectNode | null) => void;
};

function flattenTree(nodes: TreeSelectNode[]) {
  const map = new Map<string | number, TreeSelectNode>();

  const walk = (items: TreeSelectNode[]) => {
    items.forEach((item) => {
      map.set(item.id, item);
      if (item.children?.length) {
        walk(item.children);
      }
    });
  };

  walk(nodes);
  return map;
}

export function TreeSelect({
  value,
  nodes,
  placeholder = "请选择",
  disabled = false,
  clearable = true,
  className,
  onChange,
}: TreeSelectProps) {
  const [open, setOpen] = useState(false);
  const [expandedIds, setExpandedIds] = useState<Set<string | number>>(
    () => new Set(nodes.map((node) => node.id)),
  );
  const rootRef = useRef<HTMLDivElement>(null);
  const nodeMap = useMemo(() => flattenTree(nodes), [nodes]);
  const selectedNode = value == null ? null : nodeMap.get(value) ?? null;

  const toggleExpanded = (id: string | number) => {
    setExpandedIds((current) => {
      const next = new Set(current);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  return (
    <div
      ref={rootRef}
      className={cn("relative w-full", className)}
      onBlur={(event) => {
        if (
          !event.relatedTarget ||
          !event.currentTarget.contains(event.relatedTarget as Node)
        ) {
          setOpen(false);
        }
      }}
    >
      <button
        type="button"
        className={cn(
          "flex h-control-md w-full items-center justify-between gap-space-2 rounded-control border border-border bg-surface px-space-3 text-left text-sm outline-none transition-colors hover:border-border focus:border-primary disabled:cursor-not-allowed disabled:bg-neutral-background disabled:text-text-tertiary",
          open && "border-primary",
        )}
        disabled={disabled}
        onClick={() => setOpen((current) => !current)}
        aria-haspopup="tree"
        aria-expanded={open}
      >
        <span
          className={cn(
            "min-w-0 flex-1 truncate",
            !selectedNode && "text-text-tertiary",
          )}
        >
          {selectedNode?.label ?? placeholder}
        </span>
        {clearable && selectedNode && !disabled && (
          <span
            role="button"
            tabIndex={0}
            className="flex h-6 w-6 shrink-0 items-center justify-center rounded-tag text-text-tertiary hover:bg-neutral-background hover:text-text-primary"
            onClick={(event) => {
              event.stopPropagation();
              onChange(null, null);
            }}
            onKeyDown={(event) => {
              if (event.key === "Enter" || event.key === " ") {
                event.preventDefault();
                event.stopPropagation();
                onChange(null, null);
              }
            }}
            aria-label="清空选择"
          >
            <X className="h-4 w-4" aria-hidden />
          </span>
        )}
        <ChevronDown
          className={cn("h-4 w-4 shrink-0 transition-transform", open && "rotate-180")}
          aria-hidden
        />
      </button>

      {open && (
        <div className="absolute z-30 mt-space-1 max-h-72 w-full overflow-auto rounded-admin border border-border bg-surface p-space-1 shadow-admin">
          {nodes.length > 0 ? (
            <div role="tree" aria-label="树形选择">
              {nodes.map((node) => (
                <TreeSelectItem
                  key={node.id}
                  node={node}
                  level={0}
                  selectedId={value ?? null}
                  expandedIds={expandedIds}
                  onToggleExpanded={toggleExpanded}
                  onSelect={(selected) => {
                    onChange(selected.id, selected);
                    setOpen(false);
                  }}
                />
              ))}
            </div>
          ) : (
            <div className="px-space-3 py-space-6 text-center text-sm text-text-tertiary">
              暂无可选项
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function TreeSelectItem({
  node,
  level,
  selectedId,
  expandedIds,
  onToggleExpanded,
  onSelect,
}: {
  node: TreeSelectNode;
  level: number;
  selectedId: string | number | null;
  expandedIds: Set<string | number>;
  onToggleExpanded: (id: string | number) => void;
  onSelect: (node: TreeSelectNode) => void;
}) {
  const hasChildren = Boolean(node.children?.length);
  const expanded = expandedIds.has(node.id);
  const selected = selectedId === node.id;

  return (
    <div>
      <div
        className={cn(
          "flex h-control-sm items-center gap-space-1 rounded-tag text-sm",
          selected && "bg-primary/5 text-primary",
          !selected && "text-text-primary hover:bg-neutral-background",
          node.disabled && "cursor-not-allowed text-text-tertiary opacity-60",
        )}
        style={{ paddingLeft: 8 + level * 16 }}
        role="treeitem"
        aria-selected={selected}
        aria-expanded={hasChildren ? expanded : undefined}
      >
        {hasChildren ? (
          <Button
            size="icon"
            variant="ghost"
            className="h-6 w-6"
            onClick={() => onToggleExpanded(node.id)}
            aria-label={expanded ? "收起节点" : "展开节点"}
          >
            {expanded ? (
              <ChevronDown className="h-4 w-4" aria-hidden />
            ) : (
              <ChevronRight className="h-4 w-4" aria-hidden />
            )}
          </Button>
        ) : (
          <span className="h-6 w-6 shrink-0" />
        )}
        <button
          type="button"
          className="min-w-0 flex-1 truncate px-space-1 text-left outline-none disabled:cursor-not-allowed"
          disabled={node.disabled}
          onClick={() => onSelect(node)}
        >
          {node.label}
        </button>
      </div>
      {hasChildren && expanded && (
        <div role="group">
          {node.children?.map((child) => (
            <TreeSelectItem
              key={child.id}
              node={child}
              level={level + 1}
              selectedId={selectedId}
              expandedIds={expandedIds}
              onToggleExpanded={onToggleExpanded}
              onSelect={onSelect}
            />
          ))}
        </div>
      )}
    </div>
  );
}
