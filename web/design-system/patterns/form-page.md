# Standard Form Page 标准表单页

## 1. 使用场景

适用于新建、编辑、配置和密码修改等需要独立页面承载的表单流程。

## 2. 不适用场景

短流程、小范围编辑和列表行操作优先使用现有 `FormDialog`；只读信息使用 Detail Page；上传是主要任务时使用 Upload Page / Upload Section。

## 3. 页面标准结构

```text
PageHeader
└─ form
   ├─ FormSection
   │  └─ Field + Input / Select / Switch / Textarea 等控件
   └─ 固定或清晰分隔的提交操作区
```

## 4. 应复用的公共组件

复用 `PageHeader`、`FormSection`、`Field`、`ContentCard`、`Input`、`Select`、`Switch`、`Textarea` 和 `Button`。校验错误、帮助文本和必填标识沿用 `Field` 的表达。

## 5. 页面自行负责的业务内容

页面负责表单 schema、默认值、受控状态、API 提交、权限、字段联动、validation、loading、成功/失败反馈和返回行为。公共表单布局不理解业务字段。

## 6. Toolbar 与查询区规范

表单不使用列表 Toolbar。页面主操作放在稳定的提交操作区；返回、取消等导航动作位置保持可预测。筛选查询表单与编辑表单不混用。

## 7. 空状态、Loading、Error

加载初始数据时显示控件级 loading 或禁用态；提交时禁止重复提交；校验错误显示在字段下方；服务端错误使用统一错误反馈，不让整页布局跳动。

## 8. 响应式要求

默认单列；只有字段短且强相关时使用双列。操作区在窄屏下堆叠或换行，字段标签和错误文案保持可读。

## 9. 对应 Demo

`src/pages/form-example.tsx`，路由 `/forms/basic`。它展示 `FormSection`、`Field`、Zod 校验、React Hook Form 和提交状态。

## 10. 对应真实业务页面

修改密码属于该模式；通知管理是“标准表单 + 标准列表”的复合页。用户、角色、菜单、部门、字典和配置的编辑流程位于 Dialog 中，属于 FormDialog 语义，不改变独立表单模式的参考实现。

## 11. 使用边界

字段增加、校验规则变化或 API 变化属于页面业务扩展。不要因为字段排列不同就创建新的 `*FormPage` 布局组件。
