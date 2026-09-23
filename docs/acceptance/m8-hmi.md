# M8 HMI 真实链路验收

`task m8:hmi-acceptance` 是 M8-08 的显式验收入口，依据 ADR-0016 §25 编排真实 Cloud、Collector、Modbus 和浏览器场景。它不属于默认 `task check`，不依赖 M7 History/Event。

## 运行方式

在 edge-platform 根目录运行：

```text
task m8:hmi-acceptance
```

也可直接运行：

```text
node scripts/m8-hmi-acceptance.mjs
```

依赖缺失时默认输出 `SKIP M8 HMI acceptance: missing ...` 并退出 0。CI 或正式验收使用：

```text
M8_REQUIRED=1 task m8:hmi-acceptance
```

该模式会把依赖缺失、启动失败或断言失败作为失败退出。当前仓库的 web Playwright 包和 Chromium 必须可用；可用 `M8_PLAYWRIGHT_EXECUTABLE_PATH` 指定 Chromium。

## 独立环境和清理

脚本自行启动独立 Compose project，包含 PostgreSQL 17 和 Eclipse Mosquitto 2；另行构建临时 Cloud 与 sibling Edge Collector 二进制，启动真实 Modbus simulator、Collector、Cloud 和 Vite 浏览器入口。Cloud 与 Collector 使用不同的临时 PostgreSQL 数据库。

Compose project 名由本脚本 PID 和随机值生成，不接受外部 project 名覆盖。正常清理只对这个 project 执行 `down --remove-orphans --volumes`，并只停止本脚本创建的进程、移除本脚本创建的临时目录和 PTY alias。`M8_CLEANUP=0` 可保留现场供人工诊断，但会留下这个独立 project 和子进程。脚本不修改 sibling Collector 仓库文件，也不复用长期开发 Compose project。

默认 sibling 仓库位置是 `../edge-collector`。从隔离 worktree 执行时，可通过 `M8_COLLECTOR_ROOT` 指向真实 sibling Collector checkout。

默认端口为 MQTT `18888`、PostgreSQL `15438`、Cloud `18218`、Collector `18219` 和 Web `14218`。可用以下独立 M8 前缀变量覆盖：

- `M8_BROKER_PORT`、`M8_POSTGRES_PORT`、`M8_CLOUD_PORT`、`M8_COLLECTOR_PORT`、`M8_WEB_PORT`；
- `M8_COLLECTOR_ROOT`、`M8_PLAYWRIGHT_EXECUTABLE_PATH`；
- `M8_POSTGRES_USER`、`M8_POSTGRES_PASSWORD`、`M8_POSTGRES_BOOT_DATABASE`、`M8_CLOUD_DATABASE`、`M8_COLLECTOR_DATABASE`；
- `M8_JWT_SECRET`、`M8_COLLECTOR_JWT_SECRET`；
- `M8_EDGE_ID`、`M8_SOURCE_DEVICE_ID`、`M8_TOPIC_PREFIX`；
- `M8_NUMBER_ADDRESS`、`M8_BOOLEAN_ADDRESS`、`M8_BOOLEAN_BIT`、`M8_NUMBER_SCALE`；
- `M8_CLEANUP=0` 保留本次独立验收环境；`M8_REQUIRED=1` 把缺依赖转换为失败。

不要把长期部署 secret 注入临时验收脚本或提交到仓库。脚本日志会遮盖默认与显式 M8 密码、JWT secret。

## 断言范围

脚本通过临时真实 Device 建立 M4 NUMBER 和 BOOLEAN DataPoint，再通过 REST 建立 HMI canonical draft，并在真实浏览器打开 Published Runtime。计划执行以下断言：

1. 空白页面未发布时 Runtime 返回未发布错误，不读取 Draft。
2. 创建页面、保存 draft、发布后，后续 Draft 修改不改变已发布 Runtime；旧 revision 保存返回 HTTP 409。
3. 对同一已发布 revision 重试 Publish 返回同一 `versionId/versionNo`，PostgreSQL 中不增加第二个 version。
4. 用 BOOLEAN DataPoint 绑定 NUMBER-only Gauge 时，Draft 可保存但 Publish 被拒绝，页面没有 Runtime version。
5. HMI Runtime bootstrap 对重复的 `deviceId + pointKey` 绑定只返回去重后的 metadata；浏览器只向 Cloud `/api/realtime/ws` 建立一条多点连接，并对两个唯一点发送一个 batch subscribe，不连接 MQTT Broker。
6. 浏览器显示由真实 Modbus → Edge Collector → MQTT → M4 CurrentValue → M5 WebSocket 产生的 NUMBER 与 BOOLEAN，覆盖 value-display、gauge、indicator 和 switch。
7. 将真实 Collector 的 PTY channel 暂时指向本脚本独有的不存在路径，等待 CurrentValue 进入 BAD；检查最后值保留且 UI 显示“数据无效”。关闭 WebSocket 时检查 M4 quality/value 不因传输断线改变，重连后收到带 BAD quality 的 snapshot；随后恢复 PTY 并等到 GOOD。
8. 禁用 BOOLEAN DataPoint 后等待 M4 `NO_DATA`，浏览器显示不可用值；断线重连仍从 M5 snapshot 恢复 NO_DATA，再启用 DataPoint 并等待真实 GOOD。
9. 双重分发一次 HMI Button 点击，断言只产生一个 M6 Command POST 和一次真实 Modbus FC16；命令成功后等待 M4/M5 BOOLEAN CurrentValue 更新。随后通过 HMI Switch 执行反向真实命令，在 CurrentValue 更新前检查 switch 仍显示旧的现场状态，命令后等待真实 DataPoint 更新。
10. 创建只含 `hmi:run`、不含 `command:execute` 的角色：其用户仍能读 Runtime、看到实时值和 disabled control，Runtime bootstrap 返回 `canExecuteCommands=false`，M6 Command POST 返回服务端 403。
11. Cloud 重启后重新读取同一 published version，并检查浏览器收到新 snapshot 后恢复页面与 persisted CurrentValue。

每个断言只在本脚本实际运行并得到通过摘要后才能记作通过。当前实现没有声称这些场景已通过。

## 尚未覆盖的验收证据

本脚本没有覆盖 ADR-0016 §24 的完整 X6 canonical document round-trip（尤其 selection、viewport、undo/plugin state 不入库），也没有做浏览器全量兼容性回归或生产级并发压力验证。它不会用模拟 CurrentValue、手写普通 MQTT 消息、SQLite 或 M7 数据代替上述真实主链路。
