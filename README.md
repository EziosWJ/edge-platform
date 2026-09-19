# Edge Platform

Edge Platform 是与 [Edge Collector](https://github.com/EziosWJ/edge-collector) 配套的云端工业 IoT / HMI 平台。

项目采用 React + Go 的 monorepo 结构。第一阶段保持模块化单体，不拆分微服务；Edge Collector 负责现场协议、采集与控制执行，Edge Platform 负责设备语义、DataPoint、实时状态、历史、事件、Command 与 HMI。二者以 MQTT v1 contract 作为业务通信边界。

## 目录

```text
├── server/       # Go API / MQTT runtime / cloud domain
├── web/          # React 管理与 HMI 前端
├── docs/         # ADR 与架构文档
├── CONTEXT.md    # 项目领域词汇和架构约束
└── Taskfile.yml  # 开发入口
```

## 技术基线

- Backend: Go 1.26, Gin, GORM, Goose, Koanf, JWT, Prometheus, Swagger
- Frontend: React 19, TypeScript, Vite 6, Tailwind CSS, shadcn/ui, Zustand
- Production database: PostgreSQL
- Edge/Cloud transport: MQTT
- HMI editor: AntV X6（在 HMI 模块开始实施时引入）
- Realtime browser delivery: WebSocket（业务模块实施时引入）

## 开发

复制本地配置：

```sh
cp server/configs/config.dev.example.yaml server/configs/config.dev.yaml
```

常用命令：

```text
task db:migrate
task api
task web
task dev
task test
task check
```

详细架构边界见 `CONTEXT.md` 和 `docs/adr/0001-cloud-platform-scope-and-edge-cloud-boundary.md`。
