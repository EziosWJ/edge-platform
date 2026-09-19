# Edge Platform Server

Go/Gin 云端服务，继承自 `base-project-golang` 的稳定后台基础能力，并作为 Edge Platform 的 Modular Monolith composition root。

## Production database

Cloud 正式生产数据库为 PostgreSQL。仓库中暂时保留从脚手架继承的 SQLite 实现与测试代码，但 SQLite 不属于 Edge Platform 的正式生产部署目标。

## 本地开发

```sh
cp configs/config.dev.example.yaml configs/config.dev.yaml
# 修改 database.password 与 jwt.secret

docker compose -f docker-compose.dev.yml up -d --wait postgres
APP_ENV=dev go run ./cmd/migrate up --kind all
APP_ENV=dev go run ./cmd/api
```

默认 API 监听 `:8099`；Swagger 仅在开发配置启用。

也可以在仓库根目录使用：

```text
task db:migrate
task api
task backend:check
```

## Cloud domain

新业务按领域模块组织。计划中的首批模块是 `mqtt`、`edge`、`device`、`datapoint`、`realtime`、`command`、`event` 与 `hmi`。模块在首次实现时创建，不预建空 package。

Edge/Cloud 边界见 `../docs/adr/0001-cloud-platform-scope-and-edge-cloud-boundary.md`。
