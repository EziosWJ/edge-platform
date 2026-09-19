# ADR-0001: Cloud Platform Scope and Edge/Cloud Boundary

- Status: Accepted
- Date: 2026-09-19

## Context

Edge Collector 已负责现场设备轮询、Modbus/串口/TCP、Starlark 动态业务、MQTT 上下行、可靠 event/command-result 和控制执行安全边界。

新建 Edge Platform 用于承载云端设备管理、实时展示、历史、控制和 HMI。需要在开始实现 MQTT 与组态之前固定 Edge 与 Cloud 的职责，否则 Cloud 容易重新耦合现场协议和寄存器细节。

## Decision

### 1. Edge Collector 的职责

Edge Collector 继续负责：

- 现场协议与通信通道
- 设备轮询
- Starlark 设备业务逻辑
- 原始采集
- 现场控制执行
- Command Journal / safe boundary
- MQTT v1 contract 的 Edge 端实现

Cloud 不复制这些能力。

### 2. Edge Platform 的职责

Edge Platform 负责：

- Edge 管理与在线状态
- Device 云端语义
- DataPoint 模型
- CurrentValue
- Event / History
- Command 发起、状态跟踪和审计
- 告警（后续）
- HMI 编辑与运行

### 3. 通信边界

Edge Collector 与 Edge Platform 的业务通信通过 MQTT v1 contract。

正式数据链路为：

```text
Edge Collector <-> MQTT Broker <-> Edge Platform Server <-> REST/WebSocket <-> Web
```

Web 不作为正式 MQTT Subscriber，也不直接保存 Broker 凭据。

### 4. Cloud 架构

第一阶段采用 Go Modular Monolith + React SPA + PostgreSQL。

不提前拆分 mqtt-service、device-service、history-service、hmi-service 或 API Gateway。

### 5. Cloud 数据语义

Cloud 上层业务不直接以 Topic、Modbus register 或 JSON path 作为 HMI 绑定模型。

HMI 和其他上层能力统一绑定：

```text
deviceId + pointKey
deviceId + command
```

DataPoint 负责把 Edge 上报数据转换为 Cloud 可复用的设备语义。

### 6. HMI

HMI 编辑器采用 AntV X6 作为图编辑基础设施。X6 负责画布、Node/Edge、选择、拖拽、连接等编辑能力；业务数据绑定、控制权限、运行态、告警和实时值属于 Edge Platform 自己的领域能力。

持久化定义自有 HMI schema，不把 X6 JSON 直接作为长期不可替换的领域契约。

### 7. Database

Edge Platform 正式生产数据库为 PostgreSQL。

从脚手架继承的 SQLite 实现可以暂留作本地/测试兼容，但不作为 Cloud 生产兼容目标，也不限制后续 Cloud 数据模型。

## Consequences

- Cloud 可以独立演进，不需要理解现场 Modbus 细节。
- HMI 不会与 MQTT Topic 结构硬耦合。
- 第一阶段部署简单，避免过早微服务化。
- MQTT ingest、实时推送与 Cloud 业务会在同一 Server 进程内启动，需要在 app composition root 明确生命周期和 shutdown。
- 历史数据规模增长后可以针对 PostgreSQL 单独优化，而不受 SQLite 生产兼容约束。

## Initial implementation order

```text
MQTT ingest
→ Edge
→ Device
→ DataPoint
→ CurrentValue
→ WebSocket
→ Command
→ History/Event
→ HMI
```
