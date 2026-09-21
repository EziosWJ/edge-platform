# web 开发规范

## 技术栈
- React + TypeScript + Vite + Tailwind CSS + shadcn/ui + React Router + Zustand
- 包管理器：npm（以 package-lock.json 为准）

## 核心原则
- 先理解现有结构，再修改代码
- 优先复用已有组件，不过度封装
- 前端改动按根目录 `CLAUDE.md` 的验证范围选择最小反馈闭环：页面局部改动优先运行对应页面回归，暂无对应脚本时通过热更新手动确认；涉及公共组件、共享 Hook、路由、TypeScript 类型或构建配置时再运行 `npm run build`

## UI/UX
- 遵循 `design-system/MASTER.md` 规范
- 保持中性、专业、高密度、弱装饰
- 不做营销站风格，不添加无意义动画

## Page Patterns
- 新增或重构页面前，先读取 `design-system/patterns/README.md`，再读取最匹配的 Pattern 文档。
- 页面选择顺序为：直接复用现有 Pattern；在 Pattern 内做业务扩展；只有现有 Pattern 明确无法表达时，才提出新页面模式并记录原因。
- 页面业务代码负责标题、查询字段、数据、columns、操作、权限、Dialog 和业务状态；页面骨架优先复用 Pattern 对应的公共组件。
- 业务字段不同不构成重新设计页面结构的理由；特殊页面应保留语义，并在实现或页面覆盖文档中说明差异。

## 经验与注意事项
详见 `experience/LESSONS.md`
