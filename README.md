# StreamSwitch

基于 **ZLMediaKit** 的直播流媒体管理系统（后端 Go/Gin，前端 Vue 3 + Element Plus 免构建），
支持推流鉴权、播放令牌、HLS AES-128 加密、拉流代理、转推、制播（视频轮播 / 字幕 Logo 压制）、直播分享页、RBAC 与审计日志。

本仓库在原 x86_64 宝塔环境迁移包的基础上，增加了 **ARM64 原生 Ubuntu 部署**支持，并修复/改进了一批问题（见下文「改动记录」）。

## 架构

```
浏览器 / 播放器
   └─ Nginx (HTTP 51880 / HTTPS 51443)
        ├─ /          → 前端 SPA（由 livems 托管）
        ├─ /api/      → livems 后端 (Go, :8090)
        ├─ /hls-enc/  → HLS AES-128 加密代理
        ├─ /hls-key/  → 密钥下发
        └─ /live/     → ZLMediaKit (:8080)
   RTMP 推流/播放 (1935) → ZLMediaKit ──hook──→ livems
                                          └─ MySQL / Redis
```

## 目录结构

```
StreamSwitch/
├── app/
│   ├── backend/              Go 后端源码（go 1.24）、configs/config.example.yaml、assets/fonts
│   ├── frontend/dist/        前端（原生 ES Modules + UMD 库，免构建直接部署）
│   ├── deploy/               原 x86/宝塔部署模板（nginx / systemd / 迁移脚本）
│   ├── sql/schema.sql        建库脚本
│   └── docs/deploy.md        原部署文档
├── arm64/                    ARM64 参考信息（ZLM 源码 commit、apt 源备份；编译产物不入库）
├── deploy-arm64/             ★ ARM64 / 原生 Ubuntu 的部署、诊断、测试脚本（见下）
├── zlm/conf/config.ini       ZLMediaKit 配置（api.secret 为占位，部署时由 .env.local 填入）
├── systemd/                  zlm.service / livems.service
└── MIGRATION_INFO.txt        原迁移清单
```

**不入库**（见 `.gitignore`）：证书与私钥、SSH 密钥、`config.yaml`、数据库导出、素材/缓存、编译产物（`livems`、`MediaServer`、`ffmpeg`）。

## 快速开始（ARM64 Ubuntu 24.04）

> 也适用于 x86_64：把交叉编译的 `GOARCH` 改为 `amd64` 即可；原宝塔环境可直接用 `app/deploy/migrate/restore.sh`。

### 0. 本地准备（在开发机）
```bash
cp deploy-arm64/.env.example deploy-arm64/.env.local   # 填入主机 IP、数据库/管理员/ZLM 密钥、路由器信息
ssh-keygen -t ed25519 -N "" -f deploy-arm64/livems_key && ssh-copy-id -i deploy-arm64/livems_key.pub root@<HOST_IP>
# 交叉编译后端
(cd app/backend && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o ../../arm64/livems-arm64 ./cmd/server)
```

### 1. 主机依赖
```bash
apt-get install -y mysql-server redis-server nginx ffmpeg cmake libssl-dev curl rsync pkg-config nftables
```
国内主机建议先把 apt 源换成镜像（参考 `arm64/ubuntu.sources.original`）。

### 2. 上传并部署
```bash
rsync -a --exclude lib --exclude .DS_Store ./ root@<HOST_IP>:/root/livems-pkg/
scp arm64/livems-arm64 deploy-arm64/env.sh deploy-arm64/.env.local root@<HOST_IP>:/root/livems-pkg/
ssh root@<HOST_IP> 'bash /root/livems-pkg/deploy-arm64/build-zlm.sh'     # 源码编译 ZLMediaKit（约 10 分钟）
ssh root@<HOST_IP> 'bash /root/livems-pkg/deploy-arm64/deploy-arm64.sh'  # app db zlm systemd nginx start check
```
`deploy-arm64.sh` 可分步执行：`app`（应用目录 + 从模板生成 config.yaml）、`db`（建库导入）、`zlm`、`systemd`、`nginx`（站点配置 + 证书：有预置证书则安装，否则生成自签名占位）、`start`、`check`。
部署完成后登录后台 → 「域名配置」：上传真实 SSL 证书并应用、填写推流/播放域名。

### 3. 验证
```bash
bash deploy-arm64/verify.sh          # 在主机上：服务状态 / ZLM / 鉴权 / 日志
bash deploy-arm64/e2e-play.sh        # 登录 → 令牌 → HTTP-FLV / HLS 实际拉流
```
默认管理员 `admin / admin123`（`schema.sql` 初始化），首次登录请立即修改。

## deploy-arm64 脚本一览

所有脚本通过 `env.sh` 读取同目录 `.env.local`（不入库），**不含任何站点私有信息**。在主机上运行的脚本需要 `/root/livems-pkg/` 下也有 `env.sh` 与 `.env.local`。

| 脚本 | 用途 |
|---|---|
| `deploy-arm64.sh` / `build-zlm.sh` / `enable-pprof.sh` | 部署、编译 ZLM、安装新二进制并开启 pprof |
| `verify.sh` / `e2e-play.sh` | 健康检查、端到端播放测试 |
| `upload-test.sh` / `upload-bench.sh` / `profile-upload.sh` / `upload-diag.sh` | 大文件上传实测、分段基准、CPU 剖析、日志诊断 |
| `toggle-test.sh` / `toggle-diag.sh` / `task-edit-test.sh` / `reader-diag.sh` / `cert-check.sh` | 功能回归测试与诊断 |
| `purge-orphan-tokens.sh` | 清理指向已删除流的播放令牌（带备份） |
| `net-diag.sh` / `net-diag2.sh` / `wan-diag.sh` | 网口协商、链路、外网访问诊断 |
| `wan-ports-check.sh` / `wan-path-test.sh` | 用 check-host.net 海外节点测公网端口 + 主机抓包看回包路径 |
| `policy-route.sh` / `policy-route-verify.sh` | 主机默认网关为代理网关时的策略路由修复（见下） |
| `nginx-live.conf` | 原生 Nginx 站点模板（非宝塔） |

## 改动记录（相对原迁移包）

### 前端
1. 表格「操作」列按钮统一单行水平排列（13 处列加 `class-name="col-actions"`，CSS 收紧间距）。
2. 证书列表「删除」按钮始终显示，使用中时置灰并悬浮提示原因；证书上传支持**选择文件**（多选 fullchain/privkey，按 PEM 块自动归位，文件名推断证书名）。
3. **移动端适配（≤768px）**：抽屉式侧边栏、工具栏/对话框/表单/统计卡/分页窄屏布局，`store.isMobile`，`el-col` 加 `:xs="24"`。
4. 描述列表标签单行不换行；素材上传进度 100% 后显示落盘提示。

### 后端
1. **大文件素材上传失败**：`gin.FormFile` 把文件缓冲进内存（ARM 上 5 MB/s + 200% CPU）→ 改为 `MultipartReader()` 流式写盘；审计中间件不再整读 multipart 体；`ReadTimeout`→`ReadHeaderTimeout`，上传接口单独 2 小时期限；Nginx `/api/` 超时 1800s + `proxy_request_buffering off`。660 MB 上传 76s → 3.4s。
2. **编辑制播任务报 `sql: expected 21 arguments, got 23`**：`StudioTaskRepo.Update` 漏写 `quality_mode=?, crf=?`。
3. **编辑运行中的任务重启失败**：`StopTask` 等待进程回收（`done` 通道）后再返回。
4. **流「状态」开关禁用后推流不断开**：ZLM `close_stream` 要求 schema 必填 → 改用 `close_streams`；禁用即标记离线；制播任务遇输出流禁用不再无限重试。
5. **仪表盘观看人数误报**：`MarkOffline` 清零 `reader_count`/码率，同步时清理历史残留，仪表盘只累加在线流。
6. **错误提示中文化**：`response.Humanize` 把数据库/网络驱动英文报错转为中文，原文记日志。
7. 新增可选 `server.pprof_addr`。

### 部署 / 网络
- ARM64：livems 交叉编译、ZLMediaKit 源码编译（master，含 OpenSSL）、ffmpeg 用 apt 版软链到 `backend/bin/tools/`。
- **策略路由**（`policy-route.sh`）：当主机默认网关是另一台代理网关（如跑 Surge 的 Mac）时，路由器端口映射进来的连接回包会走错路被 RST。脚本用 nftables 按「入站源 MAC = 路由器」给连接打 ct mark，`ip rule fwmark → viarouter` 表让回包经路由器返回，其余出站仍走代理；NetworkManager dispatcher 持久化。`--uninstall` 回滚。

## 安全说明
- `config.yaml`、证书、SSH 密钥、数据库导出均不入库；部署脚本从 `.env.local` 读取密钥并在首次部署时由 `config.example.yaml` 生成 `config.yaml`（JWT secret 随机生成）。
- 仓库**不预设任何域名与证书**：Nginx 模板 `server_name _;` 匹配任意主机名；首次部署若未在 `certs/<CERT_NAME>/` 提供证书，脚本会生成自签名占位证书让 HTTPS 端口可启动，请部署后在后台「域名配置 → 上传证书」上传并应用自己的证书，并在「域名配置」中填写实际的推流/播放域名。
- 站点名 `SITE_NAME`（默认 `streamswitch`）决定应用目录 `/www/wwwroot/<SITE_NAME>`、Nginx 配置文件名、日志与默认证书目录，可在 `.env.local` 中更改。
- 对外请仅开放 51880 / 51443 / 1935（RTSP 554 按需），8090 / 8080 / 3306 / 6379 / 6060 严禁暴露公网。

## License
原项目未声明许可证；本仓库仅作部署与改进记录。
