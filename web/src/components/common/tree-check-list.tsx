import { ChevronDown, ChevronRight } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export type TreeCheckNode = {
  id: string | number;
  label: string;
  disabled?: boolean;
  children?: TreeCheckNode[];
};

type TreeCheckListProps = {
  nodes: TreeCheckNode[];
  checkedIds: Array<string | number>;
  disabled?: boolean;
  cascade?: boolean;
  className?: string;
  onCheckedChange: (checkedIds: Array<string | number>) => void;
};

function getNodeIds(node: TreeCheckNode): Array<string | number> {
  return [
    node.id,
    ...(node.children ?? []).flatMap((child) => getNodeIds(child)),
  ];
}

function getEnabledNodeIds(node: TreeCheckNode): Array<string | number> {
  if (node.disabled) {
    return [];
  }

  return [
    node.id,
    ...(node.children ?? []).flatMap((child) => getEnabledNodeIds(child)),
  ];
}

function getInitialExpandedIds(nodes: TreeCheckNode[]) {
  return new Set(nodes.flatMap((node) => getNodeIds(node)));
}

export function TreeCheckList({
  nodes,
  checkedIds,
  disabled = false,
  cascade = true,
  className,
  onCheckedChange,
}: TreeCheckListProps) {
  const [expandedIds, setExpandedIds] = useState<Set<string | number>>(() =>
    getInitialExpandedIds(nodes),
  );
  const checkedSet = useMemo(() => new Set(checkedIds), [checkedIds]);

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

  const setNodeChecked = (node: TreeCheckNode, checked: boolean) => {
    const next = new Set(checkedSet);
    const targetIds = cascade ? getEnabledNodeIds(node) : [node.id];

    targetIds.forEach((id) => {
      if (checked) {
        next.add(id);
      } else {
        next.delete(id);
      }
    });

    onCheckedChange(Array.from(next));
  };

  return (
    <div
      className={cn(
        "rounded-admin border border-border bg-surface p-space-2",
        disabled && "opacity-70",
        className,
      )}
      role="tree"
      aria-label="树形多选"
    >
      {nodes.length > 0 ? (
        nodes.map((node) => (
          <TreeCheckItem
            key={node.id}
            node={node}
            level={0}
            disabled={disabled}
            checkedSet={checkedSet}
            expandedIds={expandedIds}
            cascade={cascade}
            onToggleExpanded={toggleExpanded}
            onCheckedChange={setNodeChecked}
          />
        ))
      ) : (
        <div className="px-space-3 py-space-6 text-center text-sm text-text-tertiary">
          暂无可选项
        </div>
      )}
    </div>
  );
}

function getDescendantIds(node: TreeCheckNode): Array<string | number> {
  return (node.children ?? []).flatMap((child) => [
    child.id,
    ...getDescendantIds(child),
  ]);
}

function getNodeCheckState(node: TreeCheckNode, checkedSet: Set<string | number>) {
  const descendantIds = getDescendantIds(node);
  const checkedCount = descendantIds.filter((id) => checkedSet.has(id)).length;

  return {
    checked: checkedSet.has(node.id),
    indeterminate: descendantIds.length > 0 && checkedCount > 0 && checkedCount < descendantIds.length,
  };
}

function TreeCheckItem({
  node,
  level,
  disabled,
  checkedSet,
  expandedIds,
  cascade,
  onToggleExpanded,
  onCheckedChange,
}: {
  node: TreeCheckNode;
  level: number;
  disabled: boolean;
  checkedSet: Set<string | number>;
  expandedIds: Set<string | number>;
  cascade: boolean;
  onToggleExpanded: (id: string | number) => void;
  onCheckedChange: (node: TreeCheckNode, checked: boolean) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const hasChildren = Boolean(node.children?.length);
  const expanded = expandedIds.has(node.id);
  const checkState = getNodeCheckState(node, checkedSet);
  const itemDisabled = disabled || Boolean(node.disabled);

  useEffect(() => {
    if (inputRef.current) {
      inputRef.current.indeterminate = cascade && checkState.indeterminate;
    }
  }, [cascade, checkState.indeterminate]);

  return (
    <div>
      <div
        className={cn(
          "flex h-control-sm items-center gap-space-1 rounded-tag text-sm text-text-primary hover:bg-neutral-background",
          itemDisabled && "text-text-tertiary",
        )}
        style={{ paddingLeft: 8 + level * 16 }}
        role="treeitem"
        aria-expanded={hasChildren ? expanded : undefined}
      >
        {hasChildren ? (
          <Button
            size="icon"
            variant="ghost"
            className="h-6 w-6"
            disabled={disabled}
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

        <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-space-2">
          <input
            ref={inputRef}
            type="checkbox"
            className="h-4 w-4 rounded-tight border-border text-primary accent-primary"
            checked={checkState.checked}
            disabled={itemDisabled}
            onChange={(event) => onCheckedChange(node, event.target.checked)}
            aria-label={node.label}
          />
          <span className="truncate">{node.label}</span>
        </label>
      </div>
      {hasChildren && expanded && (
        <div role="group">
          {node.children?.map((child) => (
            <TreeCheckItem
              key={child.id}
              node={child}
              level={level + 1}
              disabled={disabled}
              checkedSet={checkedSet}
              expandedIds={expandedIds}
              cascade={cascade}
              onToggleExpanded={onToggleExpanded}
              onCheckedChange={onCheckedChange}
            />
          ))}
        </div>
      )}
    </div>
  );
}
