# web 脚手架开发经验与注意事项

## API 对接约定

- 统一前缀 `/api`，响应格式 `{code, message, data}`，分页 `{records, total, page, pageSize}`
- 状态字段用 `1/0` 表示启停用
- 批量删除统一用 `POST /batch-delete`（不用 DELETE + body，代理链路会丢弃 body）
- Token 滑动续期，前端无需主动刷新，只处理 401
- `isBuiltin=1` 的数据在 UI 和逻辑上都需保护，禁止删除/修改关键字段
- 所有 Controller 路径为单数形式
- 顶级部门 `parentId` 固定传 `0`，不要省略

## 组件使用模式

列表页标准结构：`PageHeader` → `SearchFilterBar` → `TableToolbar` + `DataTable` + `Pagination`

- 主操作按钮放 `PageHeader`，行操作放表格内
- `DataTable` 禁止混入 API 请求逻辑
- 新建组件的前提：同一页面 JSX 影响可读性，或多页面需复用
- 通用错误处理统一使用 `lib/api-error.ts`，不在各页面重复定义

## 字典使用规范

- 优先通过 `useDictOptions(dictCode)` Hook 调后端字典接口，不要硬编码整组选项
- `constants/dicts.ts` 只放字典编码常量和协议型值域，不存完整选项列表
- 接口失败需配置 fallback，页面仍应可用
- 不要自己猜或发明字典 code

## 文件管理流程

- 上传：调 `uploadFile` 拿 `accessUrl` → 写入业务表单
- 下载：`downloadFile(id)` 拿 Blob → 创建临时 `<a>` 触发
- 预览：仅支持 `image/*` 和 `application/pdf`
- 文件名用 `record.originalName` 而非 `storageName`
- 批量上传 `uploadFiles` 始终返回 200，需通过 `succeeded/failed` 数组区分成败

## 通用 ADR 模式

- **useListPage Hook**：封装筛选 + 分页 + 数据获取生命周期，`setPageSize` 自动重置 page 为 1
- **错误处理集中化**：`getErrorMessage` 统一在 `lib/api-error.ts`
- **状态标签单一来源**：LABEL/OPTIONS/Map 定义在类型文件中，不分散到各页面

## 菜单与路由

- 后端 `GET /api/auth/menus` 驱动侧边栏，Dashboard 等前端本地硬编码保留
- 仅渲染 `visible=1` 的菜单，同级按 `sortOrder` 排序
- `menuType=DIR` 无可见子菜单时不展示
- 后端菜单 `path` 需与前端路由完全一致（如 `/system/user`）

## 系统配置模块

- `configKey` 重复时含已逻辑删除的记录也会触发 400（数据库唯一索引）
- 后端不校验 `configValue` 是否符合 `valueType`，前端需自行校验
- `BOOLEAN` 类型只接受 `"true"/"false"` 字符串，不接受 `"1"/"0"`
- 修改时 `configKey` 必须提交（即使置灰不可改）
