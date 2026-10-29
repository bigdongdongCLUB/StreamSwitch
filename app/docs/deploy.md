# 流媒体管理系统 - 部署文档

## 架构总览

```
                        ┌─────────────────────────────────────────────┐
  浏览器/播放器          │        Nginx (51880 HTTP / 51443 HTTPS)     │
 ────────────────►     │      （内外网同端口，公网映射同端口）          │
                        │   /            -> livems SPA 静态资源        │
                        │   /api/        -> livems 后端 API  ──┐      │
                        │   /hls-enc/    -> livems HLS 加密代理 │      │
                        │   /hls-key/    -> livems 密钥下发     │ 8090 │
                        │   /live/       -> ZLMediaKit (8080)  │      │
                        └─────────────────────────────────────┼──────┘
                                                               │
        RTMP 推流/播放 (1935, TCP) ──► ZLMediaKit ──hook────►  livems
                                                               │
                                                        MySQL / Redis
```

> 端口说明：对外服务使用内外网同端口——HTTP 51880、HTTPS 51443（公网映射同端口即可）；
> RTMP 推流/播放走 1935（TCP 直连 ZLM，不经 Nginx）；8090/8080/3306/6379 仅供内网，严禁映射公网。

- **livems**：Go 后端，提供管理 API、前端静态资源服务、HLS AES-128 加密代理、ZLM hook 接收
- **ZLMediaKit**：流媒体内核（推拉流、协议转换、HLS 切片落盘）
- **Nginx**：对外统一入口（HTTPS、播放分发、反代）
- **MySQL**：业务数据持久化；**Redis**：播放令牌（TTL）

## 目录结构

```
/www/wwwroot/live.rextian.com
├── backend/                 Go 后端
│   ├── cmd/server/          入口
│   ├── configs/config.yaml  运行配置（含数据库/Redis/ZLM/JWT 机密）
│   ├── internal/            业务代码（handler/service/repository/hls/zlm...）
│   ├── bin/livems           编译产物
│   └── logs/livems.log      运行日志（systemd 追加写入）
├── frontend/dist/           前端（免构建，直接部署的产物）
│   ├── index.html
│   ├── lib/                 Vue3 / VueRouter / ElementPlus UMD
│   ├── css/  js/            应用代码（原生 ES Modules）
├── deploy/
│   ├── nginx/live.rextian.com.conf   Nginx 站点配置（部署到 /www/server/panel/vhost/nginx/）
│   ├── systemd/livems.service        systemd 单元（部署到 /etc/systemd/system/）
│   ├── scripts/deploy.sh             一键部署脚本
│   └── zlm/config.ini                ZLMediaKit 配置参考
├── sql/schema.sql           数据库初始化脚本
└── docs/deploy.md           本文档
```

## 首次部署

### 1. 数据库

```bash
mysql -uroot -p -e "CREATE DATABASE live_rextian_com DEFAULT CHARSET utf8mb4;"
mysql -uroot -p live_rextian_com < sql/schema.sql
# 创建独立账号（示例）
mysql -uroot -p -e "CREATE USER 'live_rextian_com'@'127.0.0.1' IDENTIFIED BY '<密码>'; GRANT ALL ON live_rextian_com.* TO 'live_rextian_com'@'127.0.0.1'; FLUSH PRIVILEGES;"
```

### 2. 后端配置

```bash
cp backend/configs/config.example.yaml backend/configs/config.yaml
vi backend/configs/config.yaml
```

必须修改的项：

| 配置 | 说明 |
|---|---|
| `mysql.password` | 数据库密码 |
| `jwt.secret` | 后台登录令牌签名密钥，生产务必更换 |
| `zlm.secret` | 与 ZLM `config.ini` 的 `api.secret` 一致 |
| `zlm.base_url` | ZLM HTTP API 地址（默认 `http://127.0.0.1:8080`） |
| `hls.zlm_www_dir` | ZLM HLS 落盘目录（同机部署建议配置，加密代理直读文件用） |

### 3. ZLMediaKit

参考 `deploy/zlm/config.ini`，关键项：

```ini
[api]
secret=<与 config.yaml 的 zlm.secret 一致>   # 与 livems config.yaml 的 zlm.secret 一致

[http]
port=8080

[hook]
enable=1
on_publish=http://127.0.0.1:8090/index/hook/on_publish
on_play=http://127.0.0.1:8090/index/hook/on_play
on_stream_changed=http://127.0.0.1:8090/index/hook/on_stream_changed
on_flow_report=http://127.0.0.1:8090/index/hook/on_flow_report
on_stream_not_found=http://127.0.0.1:8090/index/hook/on_stream_not_found
on_stream_none_reader=http://127.0.0.1:8090/index/hook/on_stream_none_reader
on_server_started=http://127.0.0.1:8090/index/hook/on_server_started
on_server_exited=http://127.0.0.1:8090/index/hook/on_server_exited
```

### 4. systemd 服务

```bash
cp deploy/systemd/livems.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now livems
```

### 5. Nginx

```bash
cp deploy/nginx/live.rextian.com.conf /www/server/panel/vhost/nginx/
nginx -t && nginx -s reload
```

### 6. 验证

```bash
curl http://127.0.0.1:8090/health
# 浏览器打开 https://live.rextian.com/ ，admin / admin123 登录后修改密码
```

## 日常更新

```bash
bash deploy/scripts/deploy.sh            # 完整更新（编译+重启+nginx）
bash deploy/scripts/deploy.sh --no-nginx # 仅更新后端
```

## 运维要点

- **重置管理员密码**：`backend/bin/livems -c backend/configs/config.yaml -init-admin -password <新密码>`
- **日志**：`journalctl -u livems -f` 或 `tail -f backend/logs/livems.log`
- **播放鉴权**：后台「播放设置」实时生效；无令牌请求默认 401
- **HLS 加密**：流开启加密后 HLS 地址自动切换为 `/hls-enc/<app>/<stream>.m3u8`；密钥经 `/hls-key/` 下发
- **推流鉴权**：推流地址 `rtmp://<host>:1935/<app>/<stream>?key=<push_key>`，`push_key` 在流详情中查看

## 制播管理（滚动字幕 / Logo 压制 / 视频轮播）

后台「制播管理」支持两类任务：

- **视频轮播（loop）**：素材库上传的本地视频循环播出（7x24 无人值守频道/垫片）
- **拉流压制（relay）**：对一路已有直播流叠加滚动字幕/Logo 后另发一路输出流

工作原理：任务由 livems 拉起完整功能版 ffmpeg 进程（`backend/bin/tools/ffmpeg`），
产物回推本机 ZLM（`studio.push_base`），输出流自动在流管理中登记（沿用推流鉴权/播放地址/加密/转推全部能力）。

- 无字幕/Logo 时为 `-c copy` 转封装，几乎不占 CPU；有叠加时重编码（约 1 核/1080p 路）
- 滚动字幕使用思源黑体（`backend/assets/fonts/NotoSansSC-Regular.otf`），支持中文
- 任务崩溃按重试策略自动重启；livems 重启后自动恢复启用中的任务
- 素材文件存于 `backend/data/studio/`（videos/ logos/），上传限制 4GB（Nginx `client_max_body_size 4096m`）
