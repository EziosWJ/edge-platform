# Tree List Page 树形列表页

## 1. 使用场景

适用于菜单、部门、分类等层级数据，用户的主要任务是查看层级、展开收起节点、选择节点或维护父子关系。

## 2. 不适用场景

如果左侧节点选择后需要在右侧维护另一组独立数据，使用 Tree + Table；如果数据没有层级关系，使用 Standard List。

## 3. 页面标准结构

```text
PageHeader
└─ ContentCard / DataTableCard
   └─ Tree 或表格化 Tree
      └─ 可选：当前节点摘要、节点操作
```

树形页面不强制增加 SearchFilterBar 或 Pagination；是否需要它们由数据量和业务查询方式决定。

## 4. 应复用的公共组件

复用 `PageHeader`、`ContentCard`、`DataTableCard`、`DataTable`、`StatusTag`、`Button`、`EmptyState` 和现有树选择/树勾选组件。树节点的展开、缩进和选中态不在页面之间复制视觉规则。

## 5. 页面自行负责的业务内容

页面负责树数据获取、父子关系、展开与选中状态、节点增删改、权限、节点 Dialog 和业务字段。公共组件只负责呈现结构与交互容器。

## 6. Toolbar 与查询区规范

页面级主操作放在 `PageHeader`；节点新增、刷新、展开收起等操作放在树容器附近的业务 Toolbar。需要查询时复用 `SearchFilterBar`，不要为树单独创建 Search Card。

## 7. 空状态、Loading、Error

树加载中、空树和加载失败必须有明确状态；失败状态提供页面已有的重试入口。空树文案应说明是暂无节点还是筛选后无结果。

## 8. 响应式要求

树节点文本可截断但不能破坏展开按钮和键盘焦点。多列布局在窄屏下堆叠；表格化树保留横向滚动，避免通过缩小字号隐藏层级信息。

## 9. 对应 Demo

`src/pages/examples/tree-demo.tsx`，路由 `/examples/tree`。Demo 以组织树和当前节点摘要展示树形交互。

## 10. 对应真实业务页面

菜单管理和部门管理属于该模式。它们保留各自的树形数据、父节点规则、节点操作和表格化展示差异。

## 11. 使用边界

树节点字段或业务操作不同不构成新页面模式。只有当右侧数据列表成为独立主任务时，才切换到 Tree + Table。
