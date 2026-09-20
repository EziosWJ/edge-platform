# Edge Platform Roadmap

本文件定义 Edge Platform 的正式开发阶段边界。GitHub Milestone 应与这里保持一致。

原则：

- 一个 Milestone 对应一个可独立验收的能力阶段。
- 当前 Milestone 未验收关闭前，不扩展到下一阶段。
- Matt skills 可以在当前 Milestone 内生成 spec / tickets，但不得扩大 Milestone Scope。
- 每个阶段生成的 Issue 必须关联到对应 GitHub Milestone。
- Milestone 100% Issue Closed 不代表自动验收通过；必须完成阶段 Acceptance 后再关闭 Milestone。

## M1 — MQTT Ingest Foundation

目标：Cloud Server 稳定连接 MQTT Broker，订阅并解析 Edge Collector MQTT v1 消息。

设计决策见 [ADR-0010: MQTT Ingest Runtime 与交付语义](adr/0010-mqtt-ingest-runtime-and-delivery-semantics.md)。

In Scope：
- MQTT configuration
- runtime lifecycle
- connect / reconnect / resubscribe
- MQTT v1 topic/envelope parsing
- runtime diagnostic status
- graceful shutdown
- basic metrics / integration tests

Out of Scope：
- Edge persistence
- Device
- DataPoint
- WebSocket
- Command
- HMI

Acceptance：
- Cloud 能稳定收到 Edge status/raw/event
- Broker 重启后自动恢复连接和订阅
- MQTT runtime 状态可诊断
- CI 全绿

## M2 — Edge Management

目标：建立 Cloud 侧 Edge 领域模型和在线状态。

In Scope：
- Edge model / migration
- edgeId identity
- online/offline/lastSeen
- Edge list/detail API
- Edge management page

Out of Scope：
- Device semantic model
- DataPoint
- Command

Acceptance：
- 页面能看到真实 Edge
- Edge 上下线状态变化正确
- lastSeen 正确更新

## M3 — Device Domain

目标：建立 Cloud 侧 Device 模型以及 Edge 与 Device 的关系。

In Scope：
- Device model / migration
- Edge-Device relationship
- deviceId identity
- device status
- Device list/detail API
- Device management page

Out of Scope：
- DataPoint semantic mapping
- history
- HMI

Acceptance：
- 能查看某个 Edge 下的设备
- 设备状态由 MQTT 数据正确更新

## M4 — DataPoint & CurrentValue

目标：把 Edge 原始上报转换为 Cloud 可复用的设备语义。

In Scope：
- DataPoint definition
- semantic point key
- raw -> semantic mapping
- CurrentValue
- quality / timestamp
- DataPoint API / UI

Out of Scope：
- history persistence
- HMI editor
- command

Acceptance：
- Cloud 能稳定形成如 current_a = 12.3 A 的语义数据
- 上层代码不依赖 Modbus register / MQTT topic

## M5 — Realtime WebSocket

目标：把 CurrentValue 实时推送给浏览器。

In Scope：
- WebSocket lifecycle
- subscription model
- point-level realtime delivery
- reconnect behavior
- frontend realtime store

Out of Scope：
- Command
- history
- HMI editor

Acceptance：
- Edge 数据变化后浏览器实时更新
- 浏览器正式业务页面不直接连接 MQTT Broker

## M6 — Command Loop

目标：完成 Cloud -> Edge -> Cloud 的控制闭环。

In Scope：
- Command API
- authorization / audit
- MQTT command publish
- command-result consume
- command status lifecycle
- frontend command UX

Out of Scope：
- HMI editor
- advanced workflow / scheduling

Acceptance：
- 浏览器可发起真实控制
- Edge 执行后 Cloud 收到 command-result
- 成功/失败状态可追踪和审计

## M7 — History & Event

目标：形成基础历史数据和事件查询能力。

In Scope：
- point history persistence
- event persistence
- query API
- retention baseline
- basic trend/event UI

Out of Scope：
- advanced analytics
- rule engine
- long-term data warehouse

Acceptance：
- 可查询指定 DataPoint 的时间范围历史
- 可查询设备事件
- 前端能显示基础趋势图

## M8 — HMI MVP

目标：基于已稳定的 DataPoint / Command 实现第一版组态编辑与运行。

In Scope：
- AntV X6 editor
- component palette
- page schema
- DataPoint binding
- Command binding
- edit/runtime separation
- basic industrial components

Out of Scope：
- advanced SCADA scripting
- complex animation engine
- multi-user collaborative editing

Acceptance：
- 可创建并保存 HMI 页面
- 可拖入组件并绑定真实 DataPoint
- 运行态实时显示数据
- 控件可绑定真实 Command
- HMI 不直接依赖 MQTT Topic / Modbus register

## Recommended execution order

```text
M1 MQTT Ingest
→ M2 Edge
→ M3 Device
→ M4 DataPoint & CurrentValue
→ M5 WebSocket
→ M6 Command
→ M7 History & Event
→ M8 HMI MVP
```
