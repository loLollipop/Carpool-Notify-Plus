# Carpool Notify

单用户拼车**收钱提醒**系统：Go + Gin + SQLite 提供 JSON API，前端为 React SPA（Vite + React 19 + Tailwind v4 + shadcn/ui），由 Go 进程统一托管。

## 功能

- 拼车订阅 CRUD（伪删除）
- 多席位订阅（按账号容量限制）
- 人均价格（元录入 / 分存储）
- 5 段 cron 周期（时区强制 `Asia/Shanghai`）
- 多提醒偏移（到期前 N 天，每天 09:00）
- 月历 + 到期清单；可标记本期已交费并暂停该期提醒
- 仪表盘 KPI 汇总与账单图表
- Gotify + IYUU + SMTP 邮件通知，失败最多 5 次指数退避
- 全局可编辑通知模板（运营提醒 + 客户邮件）
- Session 登录、JSON 导出
- 进程内调度器

## 快速开始

```bash
# 依赖
go mod tidy

# 配置（必做）
cp config.example.toml config.toml
# 编辑 config.toml：password、session_secret；按需填 gotify / iyuu / smtp

# 构建前端（产物输出到 web/dist/，Go 进程直接托管）
cd web/app && npm install && npm run build && cd ../..

# 在仓库根目录运行
go run ./cmd/server

# 或指定配置文件路径
go run ./cmd/server -config /path/to/config.toml
```

浏览器打开 `http://127.0.0.1:8080/login`。

### 前端开发模式

```bash
# 终端 1：Go API（:8080）
go run ./cmd/server

# 终端 2：Vite dev server（:5173，/api 与 /export 代理到 :8080）
cd web/app && npm run dev
```

### 本地测试数据

```bash
# 可选：向 ./data/carpool.db 灌入一批演示账号 / 订阅 / 账单
go run scripts/seed_test_data.go
```

## 配置文件 `config.toml`

示例见 `config.example.toml`。结构：

```toml
[server]
listen = ":8080"
db_path = "./data/carpool.db"
password = "你的登录密码"
session_secret = "足够长的随机密钥"

[gotify]
url = "https://gotify.example.com"   # 不要末尾 /，不要带 /message
token = "gotify-app-token"

[iyuu]
token = "iyuu-token"

[smtp]
host = "smtp.example.com"
port = 587                      # 常用 587 STARTTLS
username = "smtp-username"
password = "smtp-password"
from = "发件人地址"
to = "运营收钱提醒收件人，逗号分隔多个"
```

### 配置路径

| 优先级 | 来源 |
|--------|------|
| 1 | 命令行 `-config /path/to/config.toml` |
| 2 | 环境变量 `CARPOOL_CONFIG` |
| 3 | 默认 `./config.toml`（相对当前工作目录） |

### 环境变量覆盖（可选）

若设置了对应环境变量，会**覆盖** TOML 中的同名字段，便于容器/CI：

| 环境变量 | 对应 TOML |
|----------|-----------|
| `CARPOOL_PASSWORD` | `server.password` |
| `CARPOOL_SESSION_SECRET` | `server.session_secret` |
| `CARPOOL_LISTEN` | `server.listen` |
| `CARPOOL_DB_PATH` | `server.db_path` |
| `GOTIFY_URL` | `gotify.url` |
| `GOTIFY_TOKEN` | `gotify.token` |
| `IYUU_TOKEN` | `iyuu.token` |
| `SMTP_HOST` | `smtp.host` |
| `SMTP_PORT` | `smtp.port` |
| `SMTP_USERNAME` | `smtp.username` |
| `SMTP_PASSWORD` | `smtp.password` |
| `SMTP_FROM` | `smtp.from` |
| `SMTP_TO` | `smtp.to` |

`config.toml` 含密钥，已加入 `.gitignore`；请只提交 `config.example.toml`。

## 构建

```bash
cd web/app && npm run build && cd ../..
go build -o carpool-notify ./cmd/server
```

部署时请将二进制、`web/dist/` 与 `config.toml` 放在同一工作目录，或使用 `-config` 指向绝对路径。

## systemd

参考 `scripts/carpool-notify.service`。推荐：

```text
WorkingDirectory=/opt/carpool-notify
# 目录内放置 config.toml（权限 600）
```

## 联调通知

1. 在 `config.toml` 填入真实 `gotify` / `iyuu` / `smtp`  
2. 重启服务  
3. 登录 → 设置页确认渠道「已配置」  
4. 订阅勾选渠道 → 点「测试发送」

## 通知模板变量

运营提醒与客户邮件两个模板共用以下变量：

`{{.Name}}` `{{.PricePerPerson}}` `{{.CycleDesc}}` `{{.NextDueDate}}` `{{.Remark}}` `{{.TradeURL}}`
