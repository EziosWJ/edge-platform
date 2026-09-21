# CLAUDE.md

请使用中文交流。

## 项目

Edge Platform 是与 Edge Collector 配套的云端工业 IoT / HMI 平台。

```text
project/
├── web/       # React SPA
├── server/    # Go Cloud Server
├── docs/
└── CONTEXT.md
```

开始任务前先读 `CONTEXT.md` 和相关 ADR。不要把 Edge Collector 的 Modbus、串口、寄存器和 Starlark 内部概念直接扩散到 Cloud HMI 领域。

## 开发入口

优先使用根目录 Taskfile：

- `task db:migrate`
- `task api`
- `task web`
- `task dev`
- `task backend:check`
- `task frontend:lint`
- `task frontend:build`
- `task check`

Agent 验证按改动边界选择最小反馈闭环：

- Web 页面局部文案、样式或交互优先通过热更新手动确认；已有对应浏览器脚本时只运行该脚本，例如 `npm --prefix web run test:layout` 或 `npm --prefix web run test:route-loading`。
- Web 公共组件、共享 Hook、路由、类型或构建配置改动运行 `task frontend:lint`；涉及类型或构建时再运行 `task frontend:build`，并运行受影响的浏览器脚本。
- Server 单模块改动运行受影响 Go package 的测试；涉及数据库契约时运行 `task backend:integration`，涉及 MQTT Broker 时运行 `task backend:mqtt-integration`。
- 跨前后端改动、重大功能或提交/合并前运行 `task check`。

Cloud 正式生产数据库为 PostgreSQL。SQLite 仅保留本地/测试兼容，不要把它当作新 Cloud 业务的生产约束。

## 架构规则

- 第一阶段保持 Modular Monolith。
- 业务代码按领域模块组织，不创建全局 controller/service/repository 目录。
- Handler 负责 HTTP；Service 负责业务规则；Repository 负责持久化。
- MQTT 接入在 Server，不让正式 Web 业务直接连接 Broker。
- HMI 绑定 DataPoint/Command，不直接绑定寄存器或 MQTT Topic。
- 不提前引入微服务、Kafka、Redis、Gateway、Kubernetes。
- 不恢复脚手架 Demo 或伪造 Dashboard 数据。

具体边界见 `docs/adr/0001-cloud-platform-scope-and-edge-cloud-boundary.md`。

## Agent skills

### Issue tracker

Issues 以 GitHub Issues 形式存放在 `EziosWJ/edge-platform`，统一用 `gh` CLI 操作。见 `docs/agents/issue-tracker.md`。

### Triage labels

沿用五个 canonical triage 角色，标签名与角色同名。见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文（single-context）：根目录 `CONTEXT.md` + `docs/adr/`。见 `docs/agents/domain.md`。
