# GitHub 最新 ADR 与 SPEC 核对记录

- 核对日期：2026-09-22
- 仓库：[EziosWJ/edge-platform](https://github.com/EziosWJ/edge-platform)
- 远程 `main`：`981821de019833a655ac0e732901a9986c0072b8`

## 最新 ADR

当前 `docs/adr/` 的最高编号是 [ADR-0013](https://github.com/EziosWJ/edge-platform/blob/main/docs/adr/0013-datapoint-identity-raw-mapping-and-currentvalue-semantics.md)：DataPoint identity、Raw mapping 与 CurrentValue 语义，状态为 Accepted，日期为 2026-09-22。

核心决策：

- Cloud 显式创建 DataPoint，使用稳定不透明的 `dataPointId`，并以 `(deviceId, pointKey)` 作为业务唯一键；M4 只允许 `NUMBER` 与 `BOOLEAN`。
- `SourceMapping` 是唯一可以理解 Modbus register 的 Cloud 内部边界；DataPoint、CurrentValue 以及后续 Realtime、History、HMI 不得依赖 MQTT Topic、register address、Unit ID 或采集内部结构。
- CurrentValue 是每个 DataPoint 一行的当前投影，质量只有 `NO_DATA`、`GOOD`、`BAD`，使用类型安全的 NUMBER/BOOLEAN 存储，不使用统一 JSONB。
- CurrentValue 的乱序判断只使用 Cloud `ReceivedAt`；接受的新观察使 `revision` 单调递增，旧观察被抑制。
- mapping 语义变化、禁用和重新启用会把 CurrentValue 重置为 `NO_DATA/null`；旧 raw 不得在配置变化后重新填充。
- raw 是 QoS0；投影失败只能记录受限 metric/log，不能通过 MQTT 重连或伪造可靠重投解决。

来源：[ADR-0013 正文](https://github.com/EziosWJ/edge-platform/blob/main/docs/adr/0013-datapoint-identity-raw-mapping-and-currentvalue-semantics.md)。

## 最新 SPEC

最新的 M4 实施规格是开放的 GitHub Issue [#25：M4 Spec：DataPoint、Raw Mapping 与 CurrentValue](https://github.com/EziosWJ/edge-platform/issues/25)，最后更新时间为 2026-09-21T23:59:13Z，且没有追加评论。

它明确要求：

- 实现 DataPoint、SourceMapping、CurrentValue 的 PostgreSQL 持久化和迁移；支持 raw-register-snapshot/v1 到语义值的类型解码、变换、质量、时间戳和 revision 语义。
- 提供认证管理 API：`GET /api/datapoint/page`、详情 GET、创建 POST、更新 PUT，以及 enabled 状态 PUT；不提供 DELETE，并同步 Swagger/OpenAPI 和操作审计。
- 提供全局 DataPoint 管理 UI、Device→DataPoint 导航、精确过滤、创建/编辑、启停和详情；M4 不引入 WebSocket。
- 覆盖确定性 domain、PostgreSQL integration、MQTT/app、REST、Web 行为测试，以及配置变更与旧 raw 并发竞态。
- 新增真实验收入口 `task m4:datapoint-acceptance`，链路必须是 Modbus simulator → 真实 Edge Collector → raw-register-snapshot/v1 → Cloud mapping → CurrentValue → REST/UI。
- M4 关闭前必须通过 `task check`、Go race/vet、PostgreSQL/MQTT integration、受影响 Web 测试、Web lint/build、真实 M4 acceptance、`git diff --check` 和最新 GitHub Actions。
- 明确排除 CurrentValue history、WebSocket、Command、HMI、事件持久化、告警规则、Cloud 侧采集控制、Redis/Kafka、通用表达式引擎和 raw 可靠重放存储。

来源：[Issue #25 正文](https://github.com/EziosWJ/edge-platform/issues/25)。

## 一致性结论

ADR-0013 给出 M4 的稳定领域边界；Issue #25 将该边界展开为可执行的实现、API/UI、测试和验收清单。当前仓库 `main` 已同步到该 ADR 与规格对应的文档状态，下一阶段应按 Issue #25 实施 M4，不应提前引入 M5 WebSocket 或 M7/M8 能力。
