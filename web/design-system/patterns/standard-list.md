# Standard List Page 标准列表页

## 1. 使用场景

适用于普通后台管理列表：用户、角色、配置、文件、日志，以及有筛选、表格、分页和行操作的 CRUD 页面。

## 2. 不适用场景

树结构是主交互时使用 Tree List；左侧节点选择直接驱动右侧数据时使用 Tree + Table；收件箱、看板或完全不同的工作流不强行套用完整列表骨架。

## 3. 页面标准结构

```text
PageHeader
SearchFilterBar
DataTableCard
├─ TableToolbar
├─ DataTable
└─ Pagination
```

## 4. 应复用的公共组件

优先复用 `PageHeader`、`SearchFilterBar`、`DataTableCard`、`TableToolbar`、`DataTable`、`Pagination`、`EmptyState`、`StatusTag` 和 `Button`。表格卡片的 border、radius、shadow、Toolbar 分隔和分页区由 `DataTableCard` 及其组合组件提供。

## 5. 页面自行负责的业务内容

页面负责 title/description、查询字段、筛选状态、API、data、columns、selection、行操作、权限、业务 Dialog 和业务状态。公共列表组件不调用 API，也不理解业务字段。

## 6. Toolbar 与查询区规范

主操作放在 `PageHeader`；查询字段放在 `SearchFilterBar`；刷新、导出、批量删除等列表操作放在 `TableToolbar` 的 actions。Toolbar 只接收展示内容，不承载业务调用。

## 7. 空状态、Loading、Error

`DataTable` 负责统一承载 loading、错误和空数据区域。页面提供业务化的空状态文案、错误重试或权限反馈；分页只在有分页语义时展示。

## 8. 响应式要求

筛选区在窄屏下堆叠，Toolbar actions 允许换行；表格通过 `DataTable` 的横向滚动和页面已有的 `minWidth` 保证可读性。不要为响应式而改变业务列定义或行高。

## 9. 对应 Demo

`src/pages/examples/list-demo.tsx`，路由 `/examples/list`。它展示筛选、重置、查询、Toolbar、状态标签、行操作、空状态和分页。

## 10. 对应真实业务页面

用户、角色、配置、文件、登录日志、操作日志均采用该模式。字典管理是双列表变体；菜单和部门虽然使用同一列表外围，但树形数据是主要差异，应按 Tree List 记录。

## 11. 使用边界

如果只是新增筛选字段、列、Toolbar action 或 Dialog，继续扩展页面业务代码；不要创建 `CrudPage`、`GenericCrudPage`、`EntityPage` 或新的 Table Card 外观。
