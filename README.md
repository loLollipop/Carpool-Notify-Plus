# Carpool Notify Plus

Carpool Notify Plus 是面向单管理员的订阅运营管理系统，用于统一管理 ChatGPT Team 席位与 Plus 账号出租业务。系统覆盖账号与席位、客户订阅、兑换交付、续费提醒、售后处理、收入成本、经营目标、市场价格和客户关怀等日常流程。

后端使用 Go、Gin 和 SQLite 提供 JSON API，前端使用 React SPA。生产环境由同一个 Go 进程托管 API 与 `web/dist/` 静态文件。

## 核心功能

### 运营工作台

- 仪表盘汇总待处理事项、订阅状态、收入利润和席位容量
- 统计卡片支持查看对应明细，业务入口可直接跳转处理
- 订阅日志集中展示待续订、已续订、到期和账号续费事项
- 支持浅色/深色主题、中文/英文界面和金额隐私模式

### Team 账号与席位

- 管理 Team 母号、账号成本、续费状态、容量和封禁状态
- 按账号导入顺序生成稳定序号，支持查看空闲、已满、冻结和已封禁账号
- 创建订阅时自动分配可用席位，优先使用序号较小的账号
- 客户退订后席位进入冷却期，默认 7 天，也可由管理员调整或提前释放
- 下月 0 元续费账号只记录状态，不进入人工续费待办

### Team 与 Plus 订阅

- Team 席位订阅和 Plus 账号出租分别管理、统一计费
- 支持月付、季付、自定义周期以及 Plus 单月短租
- 登记客户邮箱、微信、租金、成本、起始日期和提醒周期
- 标记本期已缴费后生成账单，并滚动到下一个待缴周期
- 支持设置下周期价格；调价后的新价格从下一账期生效并成为后续基准价

### 兑换与交付

- 生成、启用、停用和删除兑换码
- 公共兑换页支持信息核对、提交进度和状态同步
- 管理员可处理或驳回兑换申请，并自动选择符合条件的空闲席位
- 兑换申请与管理后台状态自动轮询更新
- 提供独立沙盒数据库，用于不影响正式数据的流程演练

### 售后处理

- 支持账号封禁、客户退订、退款和转移席位
- 退款和转移会释放原订阅占用，退订席位按设置进入冷却期
- 已处理售后记录保留 24 小时后自动清理
- 退款金额会进入账单净收入与利润统计，避免收入重复计算

### 财务与经营分析

- 账单统计覆盖 Team 与 Plus 的收入、退款、账号成本和净利润
- 支持登记闲鱼推流等运营支出，并计入经营成本
- 按订阅、账号和月份查看收入分布与趋势
- 目标中心根据现有利润和运行速率预测目标完成时间
- 定时获取 Team 市场行情，提供价格区间、利用率和新单定价参考
- 支持系统推荐调价、人工调价、批量安排下周期价格和本轮豁免
- 客户画像合并同一客户的多席位关系，并提供分层、关怀建议和福利发放记录
- 在样本量满足条件后展示 BG/NBD 等模型的启用准备度，避免小样本过度预测

### 通知与安全

- 支持 Gotify、IYUU 和 SMTP 客户邮件
- 正常续费与调价续费使用独立邮件模板，均支持后台预览
- 到期提醒失败后最多重试 5 次并采用指数退避
- Session 登录、登录失败限流、安全响应头和请求体大小限制
- 支持 JSON 数据导出，配置文件和数据库默认不进入 Git

## 技术架构

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.23、Gin、robfig/cron |
| 数据 | SQLite（modernc.org/sqlite） |
| 前端 | React 19、TypeScript、Vite 8、Tailwind CSS 4 |
| UI 与图表 | Radix UI、shadcn/ui、Lucide、Recharts |
| 状态与表单 | TanStack Query、React Hook Form、Zod |
| 通知 | Gotify、IYUU、SMTP |

## 环境要求

- Go 1.23 或更高版本
- Node.js 22 或兼容版本
- npm

## 快速开始

```bash
# 1. 安装前端依赖并构建静态文件
cd web/app
npm ci
npm run build
cd ../..

# 2. 创建配置文件
cp config.example.toml config.toml
# 修改 password、session_secret，并按需配置通知渠道

# 3. 启动服务
go run ./cmd/server
```

浏览器打开 `http://127.0.0.1:8080/login`。

### 指定配置文件

```bash
go run ./cmd/server -config /path/to/config.toml
```

### 前端开发模式

```bash
# 终端 1：Go API，默认监听 :8080
go run ./cmd/server

# 终端 2：Vite 开发服务器，默认监听 :5173
cd web/app
npm run dev
```

Vite 会将 `/api` 和 `/export` 请求代理到 Go 服务。

### 本地演示数据

```bash
go run scripts/seed_test_data.go
```

该命令会向本地数据库写入一组演示账号、订阅和账单，请勿对生产数据库执行。

## 配置

完整示例见 [`config.example.toml`](./config.example.toml)。最小配置如下：

```toml
[server]
listen = ":8080"
db_path = "./data/carpool.db"
password = "change-me"
session_secret = "change-me-to-a-long-random-string"
```

通知渠道为可选配置：

```toml
[gotify]
url = "https://gotify.example.com"
token = ""

[iyuu]
token = ""

[smtp]
host = "smtp.example.com"
port = 587
username = ""
password = ""
from = ""
to = ""
```

### 配置路径优先级

| 优先级 | 来源 |
|--------|------|
| 1 | 命令行 `-config /path/to/config.toml` |
| 2 | 环境变量 `CARPOOL_CONFIG` |
| 3 | 当前工作目录下的 `./config.toml` |

### 环境变量覆盖

环境变量会覆盖配置文件中的同名设置：

| 环境变量 | 对应配置 |
|----------|----------|
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

`config.toml` 包含密钥，已加入 `.gitignore`。不要提交真实配置、数据库、导出数据或测试截图。

## 开发验证

```bash
# 后端测试
go test ./...

# 前端代码检查与生产构建
cd web/app
npm run lint
npm run build
```

提交前应确保上述命令全部通过。

## 生产构建与部署

```bash
cd web/app
npm ci
npm run lint
npm run build
cd ../..

go test ./...
CGO_ENABLED=0 go build -trimpath -o carpool-notify ./cmd/server
```

部署目录至少需要包含：

```text
carpool-notify
config.toml
data/
web/dist/
```

systemd 示例见 [`scripts/carpool-notify.service`](./scripts/carpool-notify.service)。生产环境建议：

- 使用独立低权限用户运行服务
- 通过 Nginx 或 Caddy 提供 HTTPS 反向代理
- 限制 `config.toml` 与 `data/` 的文件权限
- 每次部署前备份正式数据库、沙盒数据库和前端静态文件
- 切换版本后检查 systemd 状态、近期日志和 HTTP 响应

## 数据说明

- 正式数据库默认位于 `./data/carpool.db`
- 沙盒数据库与正式数据库同目录，文件名带 `.sandbox`
- SQLite 数据库、配置文件、构建产物和临时目录均已通过 `.gitignore` 排除
- 管理后台的导出功能可生成 JSON 备份，但不能替代服务器级数据库备份

## License

本项目基于 [MIT License](./LICENSE) 发布。

在保留版权声明和许可声明的前提下，可以免费使用、复制、修改、合并、发布、分发、再许可和销售本软件。软件按“原样”提供，不附带任何明示或默示担保；完整法律条款以 [`LICENSE`](./LICENSE) 文件为准。

Copyright (c) 2026 Lollipopdamn
