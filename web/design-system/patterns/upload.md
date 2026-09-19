# Upload Page / Upload Section 上传页 / 上传区块

## 1. 使用场景

适用于附件、图片、文件导入、资料上传，以及需要展示上传进度、结果或预览的页面区块。

## 2. 不适用场景

上传只是编辑表单中的一个字段时，使用表单中的 `FileUpload`；文件列表管理本身使用 Standard List，并通过业务 Dialog 进入上传流程。

## 3. 页面标准结构

```text
PageHeader
└─ ContentCard / 上传区块
   ├─ FileUpload
   ├─ 可选：批量结果 / 错误反馈
   └─ 可选：图片或文件预览
```

多个上传任务可以使用多个 `ContentCard`，但每个区块必须有明确的文件类型和状态反馈。

## 4. 应复用的公共组件

优先复用 `FileUpload`、`ContentCard`、`Button`、`StatusTag`、`EmptyState` 和统一 toast/错误处理。单文件上传的接口生命周期由 `FileUpload` 负责。

## 5. 页面自行负责的业务内容

页面负责上传场景、`businessModule`、批量编排、业务关联、权限、结果合并、预览方式和提交后的业务动作。批量上传不把业务结果协议塞进通用上传控件。

## 6. Toolbar 与查询区规范

上传页不需要列表 Toolbar；文件管理列表中的上传入口放在 `PageHeader`，上传 Dialog 的按钮区由 Dialog 语义组件负责。

## 7. 空状态、Loading、Error

必须区分未选择文件、上传中、部分成功、全部成功和失败。上传中禁用重复选择或提交；错误应靠近对应文件或上传区块显示，并保留可重试路径。

## 8. 响应式要求

上传控件和文件名在窄屏下允许换行或截断；预览区域限制在内容区内；批量结果列表可滚动，不让长文件名撑破页面。

## 9. 对应 Demo

`src/pages/examples/file-upload-demo.tsx`，路由 `/examples/file-upload`。它包含单文件、多文件和图片上传，并展示错误、结果和受保护预览。

## 10. 对应真实业务页面

文件管理使用 Standard List + 上传/编辑/预览 Dialog；个人中心使用 Detail Page + 头像上传区块。新页面应先判断上传是主任务还是表单中的一个字段。

## 11. 使用边界

文件类型、大小限制和业务关联不同不需要新建上传布局。只有上传流程本身成为独立页面任务时，才采用完整 Upload Page。
