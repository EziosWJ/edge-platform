import { Pencil, Trash2 } from "lucide-react";
import { StatusTag } from "@/components/common/status-tag";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { formatDateOnly } from "@/lib/datetime";
import type {
  ApiStatus,
  DataTableColumn,
  SystemDictDataRecord,
  SystemDictTypeRecord,
} from "@/types";

const statusMeta: Record<
  ApiStatus,
  { label: string; tone: "success" | "neutral" }
> = {
  1: { label: "启用", tone: "success" },
  0: { label: "禁用", tone: "neutral" },
};

type DictTypeColumnActions = {
  onSelect: (record: SystemDictTypeRecord) => void;
  onEdit: (record: SystemDictTypeRecord) => void | Promise<void>;
  onChangeStatus: (record: SystemDictTypeRecord, status: ApiStatus) => void;
  onDelete: (record: SystemDictTypeRecord) => void;
  selectedIds: Set<number>;
  onToggleSelect: (id: number, checked: boolean) => void;
  onToggleSelectAll: (checked: boolean) => void;
  allSelectableChecked: boolean;
  selectableCount: number;
};

type DictDataColumnActions = {
  onEdit: (record: SystemDictDataRecord) => void;
  onDelete: (record: SystemDictDataRecord) => void;
  selectedIds: Set<number>;
  onToggleSelect: (id: number, checked: boolean) => void;
  onToggleSelectAll: (checked: boolean) => void;
  allChecked: boolean;
  selectableCount: number;
};

export function createDictTypeColumns({
  onSelect,
  onEdit,
  onChangeStatus,
  onDelete,
  selectedIds,
  onToggleSelect,
  onToggleSelectAll,
  allSelectableChecked,
  selectableCount,
}: DictTypeColumnActions): DataTableColumn<SystemDictTypeRecord>[] {
  return [
    {
      title: (
        <Checkbox
          aria-label="选择当前页普通字典类型"
          checked={allSelectableChecked}
          disabled={selectableCount === 0}
          onChange={(event) => onToggleSelectAll(event.target.checked)}
        />
      ),
      key: "selection",
      align: "center",
      width: 54,
      render: (_, record) => {
        const disabled = record.isBuiltin === 1;
        return (
          <Checkbox
            aria-label={`选择字典类型 ${record.dictName}`}
            checked={selectedIds.has(record.id)}
            disabled={disabled}
            title={disabled ? "内置字典不参与批量删除" : undefined}
            onChange={(event) => onToggleSelect(record.id, event.target.checked)}
          />
        );
      },
    },
    {
      title: "字典类型",
      key: "dictType",
      width: 240,
      render: (_, record) => (
        <div>
          <div className="font-medium text-text-primary">{record.dictName}</div>
          <div className="text-xs text-text-tertiary">
            {record.dictCode} · ID {record.id}
          </div>
        </div>
      ),
    },
    {
      title: "状态",
      dataIndex: "status",
      width: 96,
      render: (value) => {
        const status = value as ApiStatus;
        const meta = statusMeta[status];
        return <StatusTag tone={meta.tone}>{meta.label}</StatusTag>;
      },
    },
    {
      title: "属性",
      dataIndex: "isBuiltin",
      width: 96,
      render: (value) =>
        value === 1 ? (
          <StatusTag tone="info">内置</StatusTag>
        ) : (
          <StatusTag tone="neutral">普通</StatusTag>
        ),
    },
    {
      title: "排序",
      dataIndex: "sortOrder",
      align: "center",
      width: 90,
      render: (value) => (
        <span className="tabular-nums">{String(value ?? 0)}</span>
      ),
    },
    {
      title: "创建时间",
      dataIndex: "createTime",
      width: 180,
      render: (value) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatDateOnly(
            typeof value === "string" ? value : value ? String(value) : "",
          )}
        </span>
      ),
    },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      align: "center",
      width: 300,
      render: (_, record) => (
        <div className="inline-flex flex-wrap items-center justify-center gap-1">
          <Button size="sm" variant="ghost" onClick={() => onSelect(record)}>
            查看项
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => void onEdit(record)}
          >
            <Pencil className="h-4 w-4" aria-hidden />
            编辑
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() =>
              onChangeStatus(record, record.status === 1 ? 0 : 1)
            }
          >
            {record.status === 1 ? "禁用" : "启用"}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="text-error hover:text-error"
            disabled={record.isBuiltin === 1}
            title={record.isBuiltin === 1 ? "内置字典不允许删除" : undefined}
            onClick={() => onDelete(record)}
          >
            <Trash2 className="h-4 w-4" aria-hidden />
            删除
          </Button>
        </div>
      ),
    },
  ];
}

export function createDictDataColumns({
  onEdit,
  onDelete,
  selectedIds,
  onToggleSelect,
  onToggleSelectAll,
  allChecked,
  selectableCount,
}: DictDataColumnActions): DataTableColumn<SystemDictDataRecord>[] {
  return [
    {
      title: (
        <Checkbox
          aria-label="选择当前页字典项"
          checked={allChecked}
          disabled={selectableCount === 0}
          onChange={(event) => onToggleSelectAll(event.target.checked)}
        />
      ),
      key: "selection",
      align: "center",
      width: 54,
      render: (_, record) => (
        <Checkbox
          aria-label={`选择字典项 ${record.dictLabel}`}
          checked={selectedIds.has(record.id)}
          onChange={(event) => onToggleSelect(record.id, event.target.checked)}
        />
      ),
    },
    {
      title: "字典项",
      key: "dictData",
      width: 220,
      render: (_, record) => (
        <div>
          <div className="font-medium text-text-primary">{record.dictLabel}</div>
          <div className="text-xs text-text-tertiary">
            {record.dictValue} · ID {record.id}
          </div>
        </div>
      ),
    },
    {
      title: "排序",
      dataIndex: "sortOrder",
      align: "center",
      width: 90,
      render: (value) => (
        <span className="tabular-nums">{String(value ?? 0)}</span>
      ),
    },
    {
      title: "备注",
      dataIndex: "remark",
      width: 180,
      render: (value) => (
        <span className="block max-w-[180px] truncate text-text-secondary">
          {String(value || "-")}
        </span>
      ),
    },
    {
      title: "创建时间",
      dataIndex: "createTime",
      width: 170,
      render: (value) =>
        formatDateOnly(
          typeof value === "string" ? value : value ? String(value) : "",
        ),
    },
    {
      title: "操作",
      key: "actions",
      nowrap: true,
      align: "center",
      width: 170,
      render: (_, record) => (
        <div className="inline-flex items-center gap-1">
          <Button size="sm" variant="ghost" onClick={() => onEdit(record)}>
            <Pencil className="h-4 w-4" aria-hidden />
            编辑
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="text-error hover:text-error"
            onClick={() => onDelete(record)}
          >
            <Trash2 className="h-4 w-4" aria-hidden />
            删除
          </Button>
        </div>
      ),
    },
  ];
}
