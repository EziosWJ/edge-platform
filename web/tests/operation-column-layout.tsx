import { Eye, Pencil, Trash2 } from "lucide-react";
import { createRoot } from "react-dom/client";
import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/common/data-table";
import type { DataTableColumn } from "@/types";
import "@/styles/globals.css";

type FixtureRow = { id: number; name: string };

const columns: DataTableColumn<FixtureRow>[] = [
  { title: "名称", dataIndex: "name", width: 180 },
  {
    title: "操作",
    key: "actions",
    align: "center",
    nowrap: true,
    width: 100,
    render: () => (
      <div className="inline-flex flex-wrap items-center justify-center gap-1">
        <Button size="sm" variant="ghost">
          <Eye className="h-4 w-4" aria-hidden />
          <span data-testid="single-action-label">查看</span>
        </Button>
        <Button size="sm" variant="ghost">
          <Pencil className="h-4 w-4" aria-hidden />
          <span data-testid="multi-action-label">编辑</span>
        </Button>
        <Button size="sm" variant="ghost">
          <Trash2 className="h-4 w-4" aria-hidden />
          删除
        </Button>
      </div>
    ),
  },
];

createRoot(document.getElementById("root")!).render(
  <div style={{ width: 220 }}>
    <DataTable columns={columns} dataSource={[{ id: 1, name: "示例记录" }]} rowKey="id" minWidth={0} />
  </div>,
);
