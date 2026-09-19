# Page Patterns 页面模式库

本目录定义后台页面的参考实现和选择规则。它描述页面的组成关系与职责分界，不是万能页面组件的配置协议；实际页面仍由业务代码负责数据、状态和业务交互。

## 模式清单

| Pattern | 中文名称 | 参考实现 | 适用方向 |
| --- | --- | --- | --- |
| [Standard List Page](./standard-list.md) | 标准列表页 | `src/pages/examples/list-demo.tsx` | 普通管理、查询、分页、批量操作 |
| [Tree List Page](./tree-list.md) | 树形列表页 | `src/pages/examples/tree-demo.tsx` | 菜单、部门、分类、层级数据 |
| [Tree + Table Page](./tree-table.md) | 左树右表页 | `src/pages/examples/tree-table-demo.tsx` | 组织树/分类树与右侧数据列表联动 |
| [Standard Form Page](./form-page.md) | 标准表单页 | `src/pages/form-example.tsx` | 新建、编辑、独立配置表单 |
| [Detail Page](./detail-page.md) | 详情页 | `src/pages/examples/detail-demo.tsx` | 只读详情、摘要、状态和操作记录 |
| [Upload Page / Upload Section](./upload.md) | 上传页 / 上传区块 | `src/pages/examples/file-upload-demo.tsx` | 附件、图片、导入和资料上传 |

这些页面同时保留在应用的“页面示例”导航中，作为开发参考实现，不是业务页面模板的替代品。

## 新页面选择规则

1. 先判断页面的主任务是列表、树、树与列表联动、表单、详情还是上传。
2. 优先选择上表中最接近的 Pattern，并复用该 Pattern 列出的公共组件。
3. 只把业务内容放入页面：标题与说明、filters、columns、API、data、行操作、权限、Dialog、selection 和业务状态。
4. 如果只是字段、columns、Toolbar action 或数据状态不同，继续使用原 Pattern，不复制新的 `*PageLayout`、`*SearchCard` 或 `*TableCard`。
5. 如果页面外围结构仍然一致但数据形态特殊，记录为 Pattern 变体；只有结构、交互和响应式行为都无法表达时，才提议新 Pattern。
6. 新 Pattern 提议必须补充使用场景、不适用场景、公共组件、状态和对应示例，并说明为何现有模式不足。

## Standard List Page 默认约定

普通后台管理列表页默认采用：

```text
PageHeader
SearchFilterBar
DataTableCard
├─ TableToolbar
├─ DataTable
└─ Pagination
```

`PageHeader`、`SearchFilterBar`、`DataTableCard`、`TableToolbar`、`DataTable` 和 `Pagination` 共同提供页面外围的一致结构。业务页面提供查询字段、Toolbar actions、columns、data、分页状态、权限和行操作，不把 API 或业务状态放进公共组件。

空数据、加载中、错误和分页属于列表页的基本状态。优先使用 `DataTable`、`EmptyState` 和 `Pagination` 的现有能力；表格在窄屏下保留横向滚动，不通过改变业务 columns 来适配。

## 当前真实页面映射

| 页面 | Pattern | 当前差异或说明 |
| --- | --- | --- |
| 用户管理 `src/pages/system/users/index.tsx` | Standard List Page | 用户筛选、选择、行操作和用户 Dialog 由页面负责 |
| 角色管理 `src/pages/system/roles/index.tsx` | Standard List Page | 角色筛选、权限分配和行操作是业务扩展 |
| 菜单管理 `src/pages/system/menus/index.tsx` | Tree List Page | 层级菜单使用表格化树形数据，保留菜单专属操作 |
| 部门管理 `src/pages/system/depts/index.tsx` | Tree List Page | 部门层级、展开和节点操作是页面状态 |
| 字典管理 `src/pages/system/dicts/index.tsx` | Standard List Page 变体 | 左右为字典类型和字典项双列表，不是树；保留双表语义 |
| 配置管理 `src/pages/system/configs/index.tsx` | Standard List Page | 配置类型、内置配置保护和编辑 Dialog 是业务差异 |
| 文件管理 `src/pages/system/files/index.tsx` | Standard List Page + Upload Section | 主体是文件列表，上传/预览通过业务 Dialog 进入 |
| 登录日志 `src/pages/system/logs/login-logs.tsx` | Standard List Page | 日志筛选、详情 Dialog 和清理操作是业务扩展 |
| 操作日志 `src/pages/system/logs/oper-logs.tsx` | Standard List Page | 日志筛选、详情 Dialog 和清理操作是业务扩展 |
| 我的通知 `src/pages/notifications.tsx` | Standard List Page 轻量变体 | 无 SearchFilterBar/Toolbar，保留通知收件箱语义 |
| 通知管理 `src/pages/system/notifications/index.tsx` | Standard Form Page + Standard List Page | 发布表单与历史列表组合在同一页面 |
| 修改密码 `src/pages/change-password.tsx` | Standard Form Page | 左侧安全提示是表单页的业务辅助区块 |
| 个人中心 `src/pages/account-profile.tsx` | Detail Page + Upload Section | 只读账号信息与头像上传组合 |

字典管理的双列表和通知管理的复合页面目前没有形成第三种稳定的外围结构，因此作为变体记录，不为单个页面新增 Pattern。

## 维护规则

- Pattern 文档是页面结构的单一参考来源；公共组件接口仍以源码为准。
- Demo 的视觉、交互和业务示例应保持稳定。组件或 Token 变更时，先确认是否改变 Pattern 的参考含义。
- 新页面出现三次以上且结构稳定的共同差异，再考虑提升为新的 Pattern；单个业务特例留在页面或 `design-system/pages/` 覆盖文档中。
