# ADR-0010: MQTT Ingest Runtime 与交付语义

- Status: Accepted
- Date: 2026-09-20

## Context

M1 要求 Edge Platform Server 稳定连接 MQTT Broker，并解析 Edge Collector MQTT v1 的 Edge status、Device status、raw 和 event。M1 不建立 Edge/Device/DataPoint/Event 持久化，却必须先确定连接生命周期、订阅恢复、消息确认和过载行为；否则后续领域模块会误把一次 MQTT 投递当成 exactly-once，或把 raw register 契约扩散为 Cloud 领域模型。

## Decision

### 范围与模块边界

- MQTT Runtime 位于 `server/internal/mqtt`，是 Modular Monolith 进程内组件，不拆成微服务。
- M1 只订阅并解析以下四个 filter，全部请求 QoS 1：

  ```text
  {prefix}/+/status
  {prefix}/+/device/+/status
  {prefix}/+/device/+/raw
  {prefix}/+/device/+/event
  ```

- raw 的发布 QoS 为 0，因此实际交付仍是 QoS 0。M1 不订阅 `command` 或 `command-result`，也不暴露通用 MQTT Publish 接口。
- MQTT v1 envelope、Topic parser、status/raw/event DTO 和 Consumer port 留在 `internal/mqtt`。未来模块通过 adapter 转换为领域输入；Edge、Device、DataPoint 和 HMI 不直接复用 raw DTO。
- M1 不新增数据库表、不持久化消息、不实现 Web 页面，也不根据 retained status 推导 Edge 在线状态。

### 配置与连接

- 支持 MQTT 5 和 MQTT 3.1.1，默认 MQTT 5；分别使用 Eclipse Paho MQTT 5 与 MQTT 3.1.1 Go 客户端。
- M1 只支持一个逻辑 Broker 和一个 active MQTT Runtime，不支持共享订阅、主节点选举或多实例消息分片。
- `mqtt.enabled` 默认 `false`。禁用时状态为 `DISABLED` 且不影响 readiness；启用时无效或缺失的静态配置使进程启动失败。
- MQTT 配置沿用 YAML、环境 YAML、`APP_` 环境变量的现有优先级，修改后重启生效；M1 不支持动态热切换。
- Broker URL 只接受 `mqtt://` 和 `mqtts://`，scheme 是 TLS 的唯一开关，URL 禁止携带 userinfo。`mqtts` 使用系统 CA，并可追加自定义 CA；客户端证书和私钥必须成对配置。不提供 `insecure_skip_verify`。
- CA、客户端证书和未加密 PEM 私钥通过受控文件路径提供；用户名和密码可以由 YAML 或环境变量提供，且不得进入日志、指标或诊断响应。
- 默认配置基线为：Topic prefix `edge`、keepalive 30 秒、连接超时 10 秒、重连区间 1–30 秒、MQTT 5 session expiry 24 小时、单条 payload 上限 1 MiB、reliable 队列 1024、raw 队列 256、Consumer 超时 5 秒、关闭超时 10 秒。

### Runtime 生命周期

- Broker 不可用、拒绝认证或订阅失败时，HTTP API 继续运行；Runtime 使用带 jitter 的指数退避无限重试，成功进入 `READY` 后重置重试次数。
- Runtime 状态为 `DISABLED`、`CONNECTING`、`SUBSCRIBING`、`READY`、`RECONNECTING`、`STOPPING`、`STOPPED`。只有连接成功且四个订阅全部确认后才进入 `READY`；可恢复错误记录在独立的最近错误字段中，不设置含糊的永久 `ERROR` 状态。
- MQTT 5 使用 `Clean Start=false` 和默认 24 小时 session expiry；MQTT 3.1.1 使用 `Clean Session=false`。每次连接成功仍显式重新订阅四个 filter，不依赖 Broker 恢复订阅。
- `client_id` 是订阅身份，绑定 Broker、协议版本和 Topic prefix。修改任一绑定项时必须使用新 Client ID，或先在 Broker 删除旧 session，避免遗留订阅继续投递旧 Topic。
- 收到终止信号后先将 Runtime 置为 `STOPPING` 并令 readiness 失败，再停止接收并在 MQTT shutdown timeout 内排空已接收消息。已接受的 QoS 1 消息完成 ACK；超时未完成的 QoS 1 消息不 ACK，留待持久 session 重投；未处理 raw 可以丢弃并计数。Runtime 正常 DISCONNECT 但不 UNSUBSCRIBE，随后关闭 HTTP，数据库最后关闭。

### 解析与接收元数据

- 强类型接入消息保留 Topic、schema、messageId、edgeId、可空 deviceId、SourceTimestamp、ReceivedAt、交付 QoS、retained 标志和 typed data。
- 必填字段、字段类型、schema、时间格式以及 Topic/envelope 身份一致性严格校验；v1 对象允许未知字段，以容纳兼容性扩展。未知 schema 或不属于当前四个 filter 的消息拒绝。
- SourceTimestamp 必须是合法 RFC3339/RFC3339Nano，解析后规范化为 UTC，但不因设备时钟偏差拒绝。messageId 必须为非空合法 UTF-8，最长 256 bytes；Cloud 不固化 Collector 当前的 ID 生成格式。
- 默认在 JSON 解析前拒绝超过 1 MiB 的 payload。解析失败、超限、未知 schema 或身份不一致的消息计入拒绝指标并限流记录日志。
- QoS 或 retained 元数据偏离合同但 payload 合法时仍交付，同时记录 `contract_violation`；不因已经到达的数据可靠性偏差再次丢弃数据。

### 确认、重复与背压

- QoS 1 使用手动 ACK。解析成功并由 Consumer 返回 `accepted` 后 ACK；Consumer 返回 `rejected` 时记录永久拒绝后 ACK；返回 `retry` 或超时时不 ACK，并重建连接以触发持久 session 重投。超时时先取消 Consumer context；旧调用退出前不得启动同一消息的重投处理，Consumer 必须及时响应取消。
- 无法重试的坏消息在记录拒绝后 ACK，避免毒消息无限循环。旧连接上的消息不得在连接世代更换后继续 ACK。
- QoS 1 消息进入独立有界 reliable 队列；队列满时不 ACK 并重建连接。QoS 0 raw 使用独立有界队列；队列满时丢弃最旧待处理 raw，保留较新数据并计数。两个队列互不挤占。
- M1 不按 messageId 去重或对消息重排，只保证各接收通道内的处理顺序，不承诺跨类型全局顺序。Consumer 必须容忍重复并保持幂等。
- 持久 session 只能降低 Cloud 短暂断线时 QoS 1 消息丢失的概率；系统不承诺 exactly-once，也不承诺 Broker 重启后保留排队消息。

### 诊断与安全

- `/health` 只表示进程存活。`/ready` 要求数据库可用，且已启用的 MQTT Runtime 处于 `READY`；MQTT 禁用时只检查其他 readiness 条件。
- `GET /api/system/mqtt/status` 返回启用状态、Runtime 状态、协议版本、连接/断开/最近消息/最近错误时间、重试次数、下一次重试时间和订阅状态，不返回 Broker URL、凭据、证书内容或底层原始错误。由于当前 Server 尚无后端 permission code 强制机制，M1 只要求登录，不虚构 `system:mqtt:query` 授权。
- 诊断接口使用稳定错误码，例如 `CONNECT_FAILED`、`AUTH_REJECTED`、`SUBSCRIBE_FAILED`、`CONNECTION_LOST`、`CONSUMER_RETRY`；脱敏后的详细错误只进入限流结构化日志。
- Prometheus 指标只使用固定枚举标签，不使用 edgeId、deviceId、完整 Topic、messageId 或错误文本等无界标签。日志不记录完整 payload，并清理、截断外部身份字段。
- M1 Cloud Broker 账号按最小权限部署：只允许订阅四个上行 filter，不允许发布，也不允许订阅 command 或 command-result。应用不负责管理 Broker 用户和 ACL。

## Acceptance

- 单元测试覆盖配置验证、Topic/envelope 解析、身份不一致、payload 上限、状态机、ACK outcome、队列过载、指标标签和优雅关闭。
- 使用 Testcontainers for Go 和固定版本 Eclipse Mosquitto 做真实 Broker 集成测试。MQTT 5 覆盖明文、TLS、mTLS；MQTT 3.1.1 覆盖明文和 TLS；两种协议都覆盖 Broker 重启、重新订阅和持久 session。
- 错误 CA、hostname 不匹配、客户端证书/私钥不配对必须失败；测试和生产均不允许跳过证书校验。
- Broker 恢复可连接后，Cloud 最迟 45 秒恢复为 `READY` 并重新收到四类测试消息。Cloud 短暂断线期间的 QoS 1 event 应通过持久 session 重投；raw QoS 0 不承诺断线期间不丢失。
- 新增独立 Taskfile 入口 `backend:mqtt-integration` 并由 CI 明确执行；默认 `task check` 不强制本地 Docker。普通 MQTT 单元测试继续属于 `go test ./...` 和 `task backend:check`。

## Consequences

- M1 在不越过领域里程碑的前提下，为后续 Edge、Device、DataPoint 和 Event 模块提供清晰的接入 seam。
- Broker 故障不会让管理 API 无法启动，但会使整体 readiness 失败并暴露可诊断状态。
- 手动 ACK、持久 session 和有界队列提高了故障恢复能力，也要求 Consumer 幂等，并增加了连接世代、超时和关闭路径的测试成本。
- 单 active runtime 限制了当前部署拓扑；需要水平扩展 MQTT 消费前，必须另行决定共享订阅、分片和跨实例一致性。
