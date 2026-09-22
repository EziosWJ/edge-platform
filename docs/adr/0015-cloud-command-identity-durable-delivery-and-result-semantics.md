# Cloud Command identity、durable delivery 与 result 语义

- Status: Accepted
- Date: 2026-09-22

M6 在现有 Edge Collector ADR-0017 的 MQTT command contract 之上完成 Cloud -> Edge -> Cloud 控制闭环。Cloud 不重新设计 Edge command runtime，而是利用 Collector 已有的 `device-command/v1`、`device-command-result/v1`、command journal、QoS1 幂等和 reliable final-result 语义。

## 1. Scope and architecture

M6 控制路径：

```text
Browser / REST
  -> Cloud Command
  -> durable command_delivery
  -> MQTT QoS1
  -> Edge Collector command journal
  -> Starlark command()
  -> Modbus
  -> device-command-result/v1
  -> Cloud Command projection
```

M6 不引入：
- HMI binding
- scheduling
- workflow engine
- cancel/replay
- CommandDefinition/schema discovery
- generic Cloud outbox
- direct Modbus control from Cloud

## 2. Cloud identity and frozen routing

HTTP/API 始终以 Cloud `deviceId` 为目标，不暴露 sourceDeviceId 作为业务控制身份。

Command 创建时解析并冻结：

- Cloud deviceId
- edgeId
- sourceDeviceId
- MQTT command topic
- command name
- canonical args/request hash
- issuedAt
- expiresAt

后续 retry 必须使用创建时冻结的 route 和完全相同的 MQTT payload，不得重新读取 Device 当前 route，也不得重新生成 issuedAt/expiresAt。

未来若 Device 出现迁移能力，已创建 Command 也不能静默改发给新来源身份。

## 3. HTTP idempotency

客户端每次真实控制先生成全局唯一 `commandId` UUID。

请求 baseline：

```json
{
  "commandId": "uuid",
  "deviceId": "cloud-device-uuid",
  "name": "close",
  "args": {},
  "ttlSeconds": 30
}
```

Cloud 使用自己的时间生成：
- issuedAt = Cloud now
- expiresAt = issuedAt + ttlSeconds

第一版 TTL：
- default 30s
- minimum 1s
- maximum 300s

`args` 必须是 JSON object。最终 `device-command/v1` payload 必须满足 Collector 现有 256 KiB command payload 限制。

Cloud idempotency hash 至少包含：
- requestedBy userId
- Cloud deviceId
- command name
- canonical args
- ttlSeconds

相同 actor + commandId + 相同 hash：
- 返回已有 Command
- 不创建新的 command_delivery
- 不产生第二次真实控制

相同 commandId 但 actor 或业务请求不同：
- HTTP 409 Conflict

Command 创建建议 HTTP 202。

## 4. Authorization and audit

M6 的真实控制必须使用 server-side authorization，不能只依赖前端隐藏按钮。

至少需要权限语义：
- `command:list`
- `command:detail`
- `command:execute`

`POST /api/command` 必须在服务器验证 execute permission。

如果平台现有 RBAC 只有 menu permissionCode 而没有后端通用 permission middleware，M6 只补一个窄的 permission authorization seam，不重写整个 RBAC。

第一次 Command 创建必须在同一个 PostgreSQL transaction 中原子写入：
- Command
- command_delivery
- operation audit

Command 生命周期由机器 result 更新，不把 ACCEPTED/FINAL 每一步伪装成用户 operation audit。

## 5. Command model

Cloud Command 是长期业务控制事实，不等同于 MQTT message。

至少保存：
- commandId
- Cloud deviceId
- frozen edgeId
- frozen sourceDeviceId
- name
- canonical/request args
- requestedBy
- issuedAt
- expiresAt
- status
- deliveryExpiredAt nullable
- edgeReceivedAt nullable
- startedAt nullable
- completedAt nullable
- resultReceivedAt nullable
- result JSONB nullable
- errorType nullable
- errorMessage nullable
- create/update metadata as needed

第一版不自动删除 Command 记录。Command retention 若以后有法规或容量要求，再单独定义。

## 6. Business status

Cloud 业务状态：

- `PENDING`
- `ACCEPTED`
- `REJECTED`
- `EXPIRED`
- `SUCCEEDED`
- `FAILED`

`PUBLISHED` 不是业务状态，因为 MQTT PUBACK 只证明 Broker 接收，不证明 Edge 收到。

允许状态迁移：

```text
PENDING
  -> ACCEPTED
  -> REJECTED
  -> EXPIRED
  -> SUCCEEDED
  -> FAILED

ACCEPTED
  -> SUCCEEDED
  -> FAILED
  -> EXPIRED
```

必须允许 `PENDING -> SUCCEEDED/FAILED`，因为 Collector 的 ACCEPTED 与 FINAL 都是独立 reliable outbox 消息，FINAL 可能先于 ACCEPTED 到达 Cloud。

终态：
- REJECTED
- EXPIRED
- SUCCEEDED
- FAILED

终态之后迟到 ACCEPTED no-op。

重复相同 FINAL 幂等。冲突的第二个 FINAL 采用 first-terminal-wins：
- 不覆盖已有终态
- ACK
- metric + structured error/contract violation

## 7. Cloud delivery semantics

M6 使用专用 `command_delivery`，不建立通用 Cloud outbox。

Command 创建 transaction 同时写入 delivery，delivery 至少保存：
- commandId
- frozen topic
- exact MQTT payload
- expiresAt
- attemptCount
- lastAttemptAt nullable
- lastError nullable
- nextAttemptAt or equivalent scheduling field

推荐 retry backoff：

```text
1s -> 2s -> 4s -> 8s -> 10s -> 10s...
```

每次 retry 必须发送完全相同的 payload。

delivery 持续到：
- 收到任意合法 Edge CommandResult
- 或 expiresAt 到达

收到合法的 ACCEPTED、REJECTED、EXPIRED、SUCCEEDED、FAILED 中任一 result 后停止进一步发送。

## 8. MQTT PUBACK

Command publish 使用 QoS1。

PUBACK 只更新 delivery attempt/transport 事实，不删除 delivery，也不把 Command 改成 ACCEPTED。

只有 Edge `device-command-result/v1` 才能改变 Command 业务状态。

如果 expiresAt 到达仍没有任何合法 Edge result：
- 停止发送
- 设置 deliveryExpiredAt
- 结束/删除短生命周期 command_delivery
- Command.status 保持 PENDING

Cloud 不因为沉默推断 Edge 已 EXPIRED、FAILED 或未执行。

如果以后迟到合法 result 到达，仍允许它更新 PENDING Command。

## 9. MQTT runtime boundary

M6 扩展现有 Platform MQTT runtime，而不是创建第二个 MQTT client。

新增能力：
- QoS1 command publish
- command-result subscription

订阅：

```text
{prefix}/+/device/+/command-result
```

业务模块只获得窄的 command publisher seam，不获得任意 topic publish API。

现有 MQTT connect/reconnect/backoff 生命周期继续复用。

## 10. CommandResult validation

Cloud 只消费通过 MQTT v1 topic/envelope 基础校验后的 `device-command-result/v1`。

收到 result 后必须校验：
- commandId 已存在
- topic/envelope edgeId == Command frozen edgeId
- topic/envelope deviceId == Command frozen sourceDeviceId
- data.name == stored command name
- status 是合法状态
- required timestamps / result / error 字段满足 wire contract

unknown commandId：
- ACK
- ignore
- metric
- 不自动创建 Command

route/name mismatch、非法 status 或稳定 contract mismatch：
- permanent reject
- ACK，避免 QoS1 poison loop
- metric + structured/security log

数据库/基础设施瞬时错误：
- retry/no ACK，沿用现有可靠 ingress 语义

Command 幂等不依赖 messageId。

## 11. Result time semantics

Cloud 与 Edge 时间事实严格区分：

Cloud generated：
- issuedAt
- expiresAt

Edge result source times：
- edgeReceivedAt（wire data.receivedAt）
- startedAt
- completedAt

Cloud MQTT observation：
- resultReceivedAt = Cloud Ingest ReceivedAt

不使用 source time 代替 Cloud receive time。

## 12. Result and error persistence

`result` 保存为 JSONB，允许：
- object
- array
- string
- number
- boolean
- null

Cloud 不执行 result，不解释为 HTML。

`error` 保存稳定：
- errorType
- errorMessage

不保存或暴露 Go stack、SQL detail、credential、secret。

CommandResult 总 payload 与 Collector 当前 final-result 可靠容量 contract 对齐，第一版按 256 KiB 上限处理。

## 13. Cloud restart recovery

Command 与 command_delivery 都持久化。

Cloud restart 后：
- PENDING 且尚未过 expiresAt、存在 delivery：继续按冻结 payload/topic 发送
- 已收到 ACCEPTED/terminal 的 Command：不得重新建立 delivery
- delivery 已到期：保持 PENDING + deliveryExpiredAt，不重新发送

Cloud restart 不改变 commandId，也不生成第二个业务 Command。

## 14. Edge and Device status are not hard gates

Edge Online / Device communication status 都是 last-known assertion，不是同步 reachability guarantee。

因此 M6 不把 Edge ONLINE 或 Device ONLINE 作为 Command 创建的硬门槛。

UI 可以显示风险提示，但真正约束迟到执行的是短 TTL + Collector 的 expiresAt 校验。

这允许：
- Edge 当前 offline 时用户仍可创建 Command
- Cloud 在 TTL 内 durable delivery
- Edge 恢复且命令仍有效时执行
- TTL 已过则 Cloud 停止发送，Collector 也会拒绝/过期过时 payload

## 15. Frontend UX

M6 不依赖 WebSocket 更新 Command。

第一版：
- POST command
- Command list/detail
- Device 页面发起通用命令
- detail 每约 2 秒 polling
- terminal 后停止 polling

执行 UI 输入：
- command name
- JSON args
- TTL

不做：
- CommandDefinition
- script command schema discovery
- cancel
- manual retry/replay
- schedule
- workflow
- HMI command binding

所谓“再次执行”必须创建新的 commandId。

## 16. Acceptance baseline

真实验收必须打通：

```text
Browser/API
-> Cloud Command + command_delivery
-> MQTT QoS1
-> real Edge Collector
-> Starlark command()
-> real Modbus simulator
-> device-command-result/v1
-> Cloud Command
-> UI
```

至少证明：
- authorized user 可执行真实控制
- unauthorized user server-side 403
- HTTP response 丢失后以同 commandId retry，只产生一次真实 Modbus 控制
- same commandId different request -> 409
- Broker 暂时不可用时 Command/delivery survive，恢复后在 TTL 内执行
- Cloud publish 后重启，恢复发送同 commandId/同 payload
- FINAL 先于 ACCEPTED 时 Cloud 直接进入正确 terminal
- late ACCEPTED 不回退 terminal
- duplicate same FINAL 幂等
- conflicting FINAL first-terminal-wins 并可观测
- Cloud 收到 ACCEPTED 后重启不会导致第二次真实执行
- unknown commandId result 不创建 Command
- route/name mismatch ACK+reject，不形成 reconnect poison loop
- delivery TTL 到期无 result -> PENDING + deliveryExpiredAt，不伪造 EXPIRED
- Edge 明确返回 EXPIRED -> Command EXPIRED
- result/error/timestamps 正确持久化和显示

## Consequences

- HTTP retry、Cloud MQTT retry、Broker QoS1 duplicate 与 Edge journal 共享同一个 commandId 幂等边界。
- Cloud 对命令进行 durable at-least-once delivery，但不把 PUBACK 或超时误解释成设备执行事实。
- Command 是长期 Cloud domain record；command_delivery 是短生命周期传输事实。
- 第一版保留通用 name + args 模型，避免提前建立 CommandDefinition/规则系统。
