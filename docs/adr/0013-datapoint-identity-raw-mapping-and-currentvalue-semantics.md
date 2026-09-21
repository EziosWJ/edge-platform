# DataPoint identity、Raw mapping 与 CurrentValue 语义

- Status: Accepted
- Date: 2026-09-22

M4 把 Edge Collector 的 `raw-register-snapshot/v1` 转换为 Cloud 可复用的语义 DataPoint 与 CurrentValue。Cloud 允许只有 mapping/config 边界理解 Modbus register；DataPoint、CurrentValue 以及后续 WebSocket、History、HMI 不得依赖 MQTT Topic、register address、Unit ID、channel 或其他 Edge 采集内部结构。

## DataPoint identity and lifecycle

DataPoint 由 Cloud 运维侧显式创建，不从 raw 自动猜测，也不要求 Edge Collector 同步 DataPoint 定义。Cloud 为每个 DataPoint 分配全局唯一、稳定、不透明的随机 UUID `dataPointId`，同时以 `(deviceId, pointKey)` 作为稳定业务唯一键。

`pointKey` 是设备内稳定语义键，例如 `current_a`、`breaker_closed`，格式限定为 `[a-z][a-z0-9_]{0,63}`。创建后不可修改；禁用后仍永久占用该 Device 下的 pointKey，防止旧 HMI、历史或外部引用静默绑定到不同语义。语义变更需要禁用旧点并创建新 pointKey。

M4 可创建的 DataPoint value type 只有 `NUMBER` 与 `BOOLEAN`。valueType 创建后不可修改；`STRING` 可保留为未来领域扩展，但 M4 不允许创建没有可靠 raw source 的 STRING 点。

DataPoint 支持创建、修改 metadata/source mapping、启用和禁用，不提供硬删除、归档或 pointKey rename。修改 `name`、`unit`、`precision` 不改变 CurrentValue；修改任何影响来源语义的 mapping 字段会重置 CurrentValue。禁用和重新启用同样重置 CurrentValue 并等待新的真实 raw。

每个 DataPoint 创建时必须同时具有完整、有效的 SourceMapping，不建立未绑定草稿点。

## SourceMapping boundary

M4 的 SourceMapping 是 Cloud 内部的采集映射配置边界。管理 API/UI 可以查看和修改 mapping，但普通 DataPoint/CurrentValue 上层消费模型不得暴露这些字段。

M4 唯一 source type 为 `MODBUS_REGISTER`，使用 `functionCode + address` 定位 raw register，不使用 block name 作为稳定身份。当前只接受 function code 3（Holding Registers）和 4（Input Registers）。

第一版 encoding：

- `UINT16`
- `INT16`
- `UINT32`
- `INT32`
- `FLOAT32`
- `BOOLEAN_BIT`

16-bit 与 32-bit numeric encoding 支持每个 register 的 byte order；32-bit encoding 额外支持 word order。BOOLEAN_BIT 必须指定 0..15 的 bitIndex。32-bit 值所需的两个连续 register 必须位于同一个 valid block 中，不允许跨 block 组合，因为跨 block 不具备同一采集成功状态和来源时间语义。

NUMBER point 使用 numeric encoding，解码后的值按固定线性变换：

`semanticValue = decodedValue * scale + offset`

scale/offset 必须为有限数值。不引入 JS、Starlark、自定义表达式或规则引擎。precision 只用于展示，不改变 CurrentValue 的实际数值，也不参与存储舍入。

BOOLEAN point 只使用 BOOLEAN_BIT；scale/offset 不参与布尔值计算。

## Raw snapshot validation

M4 只消费已通过 MQTT v1 envelope/topic identity 校验的 `raw-register-snapshot/v1`。领域 adapter 继续验证 raw data 的完整结构，包括 blocks、functionCode、valid、来源时间和 register address/value 类型；同一 functionCode/address 在同一 snapshot 中出现多次时，整条 raw 视为无效。

整个 raw 结构或语义无效时，不更新任何 CurrentValue。raw 是 QoS0，因此坏消息只记录 reject/drop 指标与限流日志，不建立可靠重投。

一条领域有效 raw 内，单个 mapping 无法形成新有效值时只影响该 DataPoint，不影响同一 snapshot 中其他点。以下情况产生该点的 BAD：

- matching block `valid=false`；
- mapping address 在 snapshot 中不存在；
- 所需 register 为 null 或缺失；
- 32-bit source 不完整或跨 block；
- decode 或线性 transform 失败。

raw 的 communicationStatus 不用于推导 CurrentValue quality。DeviceStatus、EdgeStatus、MQTT Runtime 状态和消息沉默也不直接修改 DataPoint quality。

若 raw 的 `(edgeId, sourceDeviceId)` 无法解析到 M3 已登记 Device，则忽略该 raw；raw 不创建 Edge、Device 或 DataPoint，也不做 retained/replay 补偿。

## CurrentValue model

DataPoint 是低频 configuration/control plane；CurrentValue 是高频 data plane。M4 将 CurrentValue 持久化为独立于 DataPoint metadata 的 1:1 current projection，而不是把高频值写入 DataPoint 行。

CurrentValue 的领域字段：

- dataPointId
- value
- quality
- sourceTimestamp
- observedAt
- revision

quality 只有：

- `NO_DATA`：当前 mapping 尚未得到过可判定的 raw 结果，或配置/enable 状态刚重置；
- `GOOD`：当前 mapping 在一个 valid block 内完整解码并成功 transform；
- `BAD`：Cloud 已接受一条领域有效 raw，但当前 mapping 无法形成新有效值。

M4 不引入 `UNCERTAIN`。

CurrentValue 内部按 DataPoint valueType 使用类型安全列保存 NUMBER/BOOLEAN，而不是把 value 统一存入 JSONB。REST/后续上层接口仍以统一 `value` 字段返回 number、boolean 或 null。

DataPoint 创建时同时创建 CurrentValue：

- value = null
- quality = NO_DATA
- sourceTimestamp = null
- observedAt = null
- revision = 0

GOOD 时：

- value = 本次语义值；
- quality = GOOD；
- sourceTimestamp = 产生该值的 matching block `lastSuccessAt`；
- observedAt = 本次 raw 的 Cloud `ReceivedAt`。

BAD 时保留最近一次 GOOD 的 value 与该 value 对应的 sourceTimestamp，只更新 quality、observedAt 和 revision。若从未 GOOD，则 value/sourceTimestamp 保持 null。

因此 `Device OFFLINE + CurrentValue GOOD` 是合法的最后已知投影；若 Cloud 没收到能证明该点失败的 raw，Cloud 不自行把 quality 改成 BAD。

## Ordering, atomicity and revision

CurrentValue current-state 排序只使用 Cloud MQTT Ingest 的 `ReceivedAt`：

- incoming ReceivedAt > current observedAt：应用；
- incoming ReceivedAt < current observedAt：no-op；
- incoming ReceivedAt == current observedAt：数据库后执行的 statement 胜出。

SourceTimestamp、block lastSuccessAt、MQTT envelope timestamp 和 messageId 都不用于重排或去重。

每次接受一个新的 point projection，revision 单调 +1；即使 value 与上一值相同，只要这是新的 accepted Cloud observation，observedAt 前进且 revision 增加。被 older ReceivedAt 抑制的 projection 不增加 revision。

修改 source mapping、禁用或重新启用 DataPoint 时，在同一配置事务中把 CurrentValue 重置为 NO_DATA/null，并使 revision +1。实现必须通过 mapping/config generation、effective-time guard、行级序列化或等价机制保证：配置重置后，变更之前已经进入处理流程的旧 raw 不得使用新 mapping 重新填充 CurrentValue；只有配置生效后的新 raw 才能结束 NO_DATA。

同一 raw snapshot 对一个 Device 的所有 enabled DataPoint 先基于一个一致的 mapping snapshot 计算，再在一个数据库事务中提交。不能出现同一个 raw observation 只更新了一部分 DataPoint 的持久化状态。每个 CurrentValue 仍独立执行 ReceivedAt monotonic guard。

若整个 batch 遇到数据库失败，事务回滚。由于 raw 是 QoS0，该 delivery 的持久化失败只记录固定维度 metric 和结构化日志，等待后续 raw 自愈；不得通过 MQTT reconnect/no-ACK 伪造可靠重试或形成重连风暴。

## Configuration reset semantics

以下修改只改变 metadata，CurrentValue 保留：

- name
- unit
- precision

以下修改改变来源语义并重置 CurrentValue：

- functionCode
- address
- encoding
- wordOrder
- byteOrder
- bitIndex
- scale
- offset

pointKey 和 valueType 不可修改。

禁用 DataPoint 后不再消费 raw 更新该点；重新启用后保持 NO_DATA，直到配置生效后下一条真实 raw 到达。

## API and UI boundary

M4 提供全局 DataPoint 管理资源。建议 REST baseline：

- `GET /api/datapoint/page`
- `GET /api/datapoint/:dataPointId`
- `POST /api/datapoint`
- `PUT /api/datapoint/:dataPointId`
- `PUT /api/datapoint/:dataPointId/enabled`

列表至少支持 deviceId、pointKey、valueType、enabled、quality 精确过滤。管理列表/详情组合返回 DataPoint metadata、CurrentValue；管理写 DTO 可以包含 SourceMapping。

后续 M5 realtime、M7 history 和 M8 HMI 只依赖 Cloud 语义模型，例如 `deviceId + pointKey + valueType + unit + value + quality + sourceTimestamp + observedAt + revision`，不得依赖 SourceMapping、Modbus register 或 MQTT Topic。

M4 管理写操作复用现有认证与操作审计基础，不在本阶段重新设计 RBAC 权限体系。

## Acceptance baseline

M4 的真实验收使用 PostgreSQL、Mosquitto、Cloud Server、真实 Edge Collector 与真实 Modbus simulator，必须打通：

`Modbus register -> Edge Collector raw -> Cloud mapping -> DataPoint -> CurrentValue -> REST/UI`

至少证明：

- NUMBER point 可形成例如 `current_a = 12.3 A / GOOD`；
- BOOLEAN_BIT 可形成真实 boolean / GOOD；
- 采集失败后保留最后 GOOD value/sourceTimestamp，同时 quality=BAD、observedAt/revision 前进；
- 恢复采集后重新 GOOD；
- source mapping 变化立即 NO_DATA/null，旧 in-flight raw 不得重新填充，后续真实 raw 才形成新值；
- disable/enable 需要新的真实 raw 才恢复；
- 32-bit、word/byte order、bit mapping 和线性 transform 有确定性测试；
- 同一 raw 多点批量投影在 PostgreSQL 中具备事务原子性；
- duplicate/older/equal ReceivedAt 按上述规则处理；
- unknown Device raw 不创建任何领域对象；
- raw 不自动创建 DataPoint；
- PostgreSQL schema 不引入 history、WebSocket、Command、HMI 或通用消息 staging。

真实链路不依赖概率性数据库竞态或 MQTT QoS0 重投；并发、乱序、批事务与配置变更竞态由确定性的 PostgreSQL integration tests 覆盖。

## Consequences

- Cloud 可以在受限 mapping 边界理解 Modbus register，从而继续使用现有 raw v1 contract；不需要在 M4 修改 Edge MQTT contract。
- DataPoint/HMI/Realtime 获得稳定语义身份，不依赖 Edge 内部采集配置。
- CurrentValue 的 quality 与 Device communication status 是两种不同事实，上层可以组合展示但不得互相覆盖。
- revision 为 M5 提供稳定的单点新旧判断，但 M4 不实现 WebSocket。
- M4 不提供历史；CurrentValue 始终只是当前投影。
