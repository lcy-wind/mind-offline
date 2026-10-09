# 精神离职 · Mind Offline

> 人可以不清醒，饭不能不吃。

一个打工人精神状态主题的虚拟点餐实验。没有真实食品、支付、充值或配送。

- 顾客试玩：<https://mind-offline.duckdns.org/>
- 店长后台：<https://mind-offline.duckdns.org/admin/>

## 技术与目录

| 目录 | 技术 | 用途 |
| --- | --- | --- |
| `miniapp/` | uni-app + Vue 3 + TypeScript | 同一份顾客端代码构建网页和微信小程序 |
| `admin/` | Vue 3 + Vite + TypeScript | 手机/电脑均可使用的店长后台 |
| `server/` | Go 标准库 HTTP + pgx | API、事务、身份校验、静态资源托管 |
| `deploy/` | Docker Compose + Caddy | 运行配置、HTTPS 入口示例 |
| `scripts/` | Python 标准库 | 构建、部署、API 冒烟检查 |

Go 程序嵌入两套编译后的网页。服务器只增加一个应用容器；PostgreSQL、Caddy 复用现有实例。PostgreSQL 中使用独立的 `mind_offline` 数据库和角色。应用监听 `127.0.0.1:18082`，经 Caddy 对外提供 HTTPS。

## 第一版功能

- 9 款抽象菜品、分类、售罄显示、精神状态规格、购物袋和备注。
- 访客首次获得 300 精神值，每天（北京时间）可领取 100。
- 服务器核算价格，事务扣款，幂等下单；失败不扣款，取消只退款一次。
- 顾客仅能查看自己的订单，页面显示时每 8 秒更新菜单、余额和订单。
- 订单流程：待接单 → 制作中 → 待取餐 → 已完成；前两步可取消。
- 随机抽象评语和可保存的 PNG 小票。
- 店长口令登录、接单/出餐/取消、菜品新增/编辑/售罄、今日统计。

访客凭证保存在本地存储，服务端只保存凭证哈希；清空浏览器/小程序数据后不能恢复原身份。暂不支持微信登录和多设备同步。后台会话有效期 12 小时，保存在标签页会话存储，可主动退出。全站纯模拟，精神值没有现金价值。

## 本地开发

环境：Go 1.26+、Node.js 22.12+ 或 24、npm、可连接的 PostgreSQL。前端依赖锁定在各自的 `package-lock.json`。

```sh
npm --prefix miniapp ci
npm --prefix admin ci
cd server
go mod download
```

参考 `server/.env.example` 设置 `DATABASE_URL`、`ADMIN_PASSWORD`（至少 16 位），再运行：

```sh
cd server
go run .
```

Go 启动时创建缺失的表、写入初始菜品；已有菜品不会被覆盖。开发时另开终端：

```sh
npm --prefix miniapp run dev:h5
npm --prefix admin run dev
```

两套 Vite 开发服务均将 `/api` 代理到本地 `127.0.0.1:18082`。顾客网页地址以命令输出为准；管理网页使用 `/admin/` 路径。首次从源码启动 Go 时尚无静态网页，使用 Vite 或先执行完整构建。

## 完整构建

```sh
python3 scripts/build.py
```

依次执行顾客端类型检查、H5 构建、微信小程序构建、后台类型检查和构建、Go 测试与 vet，最后交叉编译 Linux amd64 静态程序到 `.deploy/mind-offline`。所有构建均在本地完成。

## 微信小程序

目前没有用户 AppID，交付以 H5 网页试玩为准；编译产物可在 `miniapp/dist/build/mp-weixin` 查看，尚未完成真机预览或微信发布。

取得 AppID 后：

1. 在 `miniapp/src/manifest.json` 的 `mp-weixin.appid` 填写自己的 AppID。
2. 执行 `npm --prefix miniapp run dev:mp-weixin`，使用微信开发者工具导入 `miniapp/dist/dev/mp-weixin`。
3. 接口域名固定为 `https://mind-offline.duckdns.org`（见 `miniapp/src/lib/api.ts`）。实际真机/发布前按微信当时要求配置合法域名等条件。
4. 在真机验证登录、网络、小票保存、界面和生命周期。目前只有访客身份；不要将 AppSecret 放进前端。

## 服务器部署

初始环境约定：已有 Caddy 容器 `jinx-https`、PostgreSQL 容器 `jinx-postgres`，均采用 host 网络。复用实例但不修改已有业务数据库。

一次性准备：由管理员创建专用数据库/角色，把运行环境写入服务器 `/opt/mind-offline/production.env`，权限设为 `600`。内容参考 `server/.env.example`，建议再设 `GOMEMLIMIT=140MiB`、`GOMAXPROCS=2`。

构建后运行：

```sh
python3 scripts/deploy.py
```

脚本通过已有 SSH 认证部署，使用带时间戳的镜像标签，健康检查失败会尝试恢复上一镜像。不保存 SSH 密码。需事先准备环境文件与 Caddy 域名入口（`deploy/Caddyfile.snippet`）。首次配置 Caddy 时备份原文件、验证配置后热加载，不重启现有服务。

容器限制：192 MiB 内存、0.75 CPU、最多 64 进程；非 root、只读文件系统、移除 Linux capabilities；日志按 5 MiB × 3 轮转。数据库连接池最多 5 个连接。限制仅针对 Go 容器，不涵盖已有 PG/Caddy；不代表承诺并发容量。

店长口令保存在本机 `.secrets/店长登录信息.txt`，该目录被 Git 忽略。部署后可使用该文件中的口令登录后台。不要把 `.secrets/` 或服务器的环境文件提交到仓库。

## 验证

```sh
cd server
go test ./...
go vet ./...
```

针对运行中的服务执行：

```sh
BASE_URL=https://mind-offline.duckdns.org ADMIN_PASSWORD='<店长口令>' python3 scripts/smoke.py
```

请通过环境注入真实口令，避免保存在 shell 历史中。冒烟脚本检查鉴权、访客隔离、补给并发、重复请求、非法数量、价格篡改、余额不足、退款幂等性、订单流程、售罄和历史价格。它创建临时访客/菜品/订单，最后在 `.deploy/smoke-cleanup.sql` 输出只针对这些数据的清理 SQL，需在测试数据库执行。不要把测试流量当作压力测试结果。

## 数据与维护

- 业务数据都在 PostgreSQL；更新应用容器不会删除订单。
- 正式长期使用前，应设置数据库定时备份并存到服务器之外；当前不自动配置外部备份。
- UI 展示最近 100 笔订单；本版本无自动出餐、支付、真实微信登录、图片上传或多门店。
- 菜品使用 Emoji 作为插图，不依赖外部图片服务。不同系统的 Emoji 外观可能不同。
- PostgreSQL 和系统版本沿用现有环境，未在本项目部署中升级。
