# Tree + Table Page 左树右表页

## 1. 使用场景

适用于组织树 + 人员、分类树 + 数据、设备树 + 设备列表等场景：左侧选择上下文，右侧显示并操作该上下文下的数据。

## 2. 不适用场景

没有右侧独立数据集时使用 Tree List；普通全宽筛选表格使用 Standard List；左侧只是静态说明或个人摘要时不要伪装成树表页面。

## 3. 页面标准结构

```text
PageHeader
└─ responsive grid
   ├─ ContentCard：树/分类选择
   └─ SearchFilterBar
      DataTableCard
      ├─ TableToolbar
      ├─ DataTable
      └─ Pagination
```

## 4. 应复用的公共组件

复用 `PageHeader`、`ContentCard`、`SearchFilterBar`、`DataTableCard`、`TableToolbar`、`DataTable`、`Pagination`、`StatusTag` 和 `Button`。左右区域的 Card 视觉与标准列表页保持一致。

## 5. 页面自行负责的业务内容

页面负责左树数据、当前选中节点、右侧查询和数据请求、columns、selection、权限、行操作和 Dialog。公共组件不感知左树与右表之间的业务关联。

## 6. Toolbar 与查询区规范

页面主操作放在 `PageHeader`；右侧查询字段放在 `SearchFilterBar`；当前节点名称、统计数量和刷新/新增操作放在右侧 `TableToolbar`。Toolbar actions 不直接调用 API。

## 7. 空状态、Loading、Error

左树和右表分别表达加载失败与空状态。未选择节点、节点下无数据和查询无结果应使用不同文案；右侧表格沿用 `DataTable` 的 loading/error/empty 能力。

## 8. 响应式要求

桌面端采用左右两列，窄屏下先显示树选择，再显示筛选和表格。左侧宽度只服务于节点可读性，右侧表格保留横向滚动，不压缩业务列。

## 9. 对应 Demo

`src/pages/examples/tree-table-demo.tsx`，路由 `/examples/tree-table`。Demo 展示组织分类选择、右侧成员筛选、Toolbar、表格和分页。

## 10. 对应真实业务页面

当前没有完全相同的真实“左树右表”业务页。字典管理的字典类型 + 字典项是相近的双列表变体，但左侧不是树，因此继续按 Standard List 变体维护。

## 11. 使用边界

左右区域的业务字段不同不需要新建布局组件。只有左侧上下文选择和右侧数据列表同时成立时才采用本模式；否则选择更简单的 Tree List 或 Standard List。
