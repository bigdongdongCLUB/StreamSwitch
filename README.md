# StreamSwitch

基于 **ZLMediaKit** 的直播流媒体管理系统：后端 Go（Gin），前端 Vue 3 + Element Plus（免构建）。
支持推流鉴权、播放令牌、HLS AES-128 加密、拉流代理、转推、制播（视频轮播 / 字幕 Logo 压制）、直播分享页、RBAC 与审计日志，后台适配手机端。

## 一键部署

**支持系统**：Ubuntu 22.04 / 24.04、Debian 12 及以上；**CPU 架构**：x86_64、ARM64（自动识别）。需要 root 权限。

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/bigdongdongCLUB/StreamSwitch/main/install.sh)
```

运行后出现中文菜单：

```
  ╔══════════════════════════════════════════╗
  ║     StreamSwitch 流媒体管理系统 一键部署    ║
  ╚══════════════════════════════════════════╝
  状态：未安装

   1. 安装 / 升级
   2. 端口设置
   3. 卸载
   4. 重置管理员密码
   0. 退出
```

### 1. 安装 / 升级
- **首次执行 = 安装**：自动识别架构并下载对应安装包，自动安装依赖（nginx、redis、ffmpeg、MySQL/MariaDB，已有则复用），建库建账号、生成全部密钥、配置 systemd 与 Nginx，启动并做健康检查。
  过程中只需回答几个问题（直接回车即用默认值）：是否切换国内 apt 镜像、HTTP 端口（51880）、HTTPS 端口（51443）、RTMP 端口（1935）、RTSP 端口（554，填 0 关闭）；端口被占用会提示重新输入。若本机 MySQL 的 root 无法免密登录、或 Redis 设置了密码，会提示输入。
- **再次执行 = 升级**：保留数据库、素材、证书与端口设置，先把当前程序、配置和数据库备份到 `/opt/streamswitch/backup/<时间>/`（保留最近 3 份），再替换程序并自动升级数据库结构。
- 部署完成后打印**后台地址**、推流地址与需要在路由器映射的端口。

### 首次登录：注册超级管理员
系统**没有任何默认账号**。安装完成后打开后台地址，页面会直接要求注册账号——**第一个注册的账号即为超级管理员**；注册完成后该入口自动关闭，其他用户请在「用户管理」中创建。

### 2. 端口设置
显示当前端口，输入新值（回车保持不变）后自动更新 Nginx / ZLMediaKit / 后端配置并重启。修改后记得同步路由器端口映射。

### 3. 卸载
停止并删除服务、Nginx 站点与程序目录。会询问是否同时删除数据库与素材：
- 选「否」（默认）：数据库保留在本机，素材、证书和一份数据库备份移到 `/opt/streamswitch-data-<时间>/`，以后重新安装时可一键恢复；
- 选「是」：彻底删除数据库、Redis 缓存与全部数据。

依赖软件（nginx / redis / mysql / ffmpeg）不会被卸载。

### 4. 重置管理员密码
列出全部后台用户，选择用户名（默认第一个超级管理员）后输入新密码；直接回车则随机生成并显示。该账号若被禁用会同时启用。

### 安装后管理
```bash
sudo streamswitch                     # 打开菜单
sudo streamswitch install             # 也可以直接带参数：install | port | uninstall | reset-password | status
```

### 无人值守部署
所有提问都可用环境变量预先给出，配合 `SS_YES=1` 全程不交互：

| 变量 | 说明 |
|---|---|
| `SS_YES=1` | 采用默认值，不提问（卸载时视为已确认） |
| `SS_HTTP_PORT` `SS_HTTPS_PORT` `SS_RTMP_PORT` `SS_RTSP_PORT` | 端口（RTSP 填 0 关闭） |
| `SS_MYSQL_ROOT_PASSWORD` / `SS_REDIS_PASSWORD` | 本机 MySQL root 密码 / Redis 密码（需要时） |
| `SS_APT_MIRROR=tuna` | apt 切换为清华镜像 |
| `SS_PACKAGE` / `SS_VERSION` / `GH_PROXY` | 指定本地安装包或 URL / 指定版本 / GitHub 下载加速前缀 |
| `SS_FORCE=1` | 版本相同也重新安装 |
| `SS_PURGE=1` | 卸载时删除数据库与素材 |
| `SS_RESET_USER` `SS_RESET_PASSWORD` | 重置密码的目标用户与新密码 |

```bash
curl -fsSL -o install.sh https://raw.githubusercontent.com/bigdongdongCLUB/StreamSwitch/main/install.sh
sudo SS_YES=1 SS_HTTP_PORT=8880 bash install.sh install
```

**离线安装**：在 [Releases](https://github.com/bigdongdongCLUB/StreamSwitch/releases) 下载对应架构的 `streamswitch-linux-<amd64|arm64>.tar.gz`，解压后执行 `sudo bash streamswitch/install.sh`。

## 部署说明

### 安装目录与服务
```
/opt/streamswitch/
├── backend/bin/livems           后端
├── backend/configs/config.yaml  后端配置（由脚本生成）
├── backend/data/                素材（studio/）、HLS 密钥与缓存
├── backend/logs/livems.log      后端日志（logrotate 每日轮转，保留 7 天）
├── frontend/dist/               前端
├── zlm/                         ZLMediaKit（bin/ conf/ www/ log/）
├── certs/                       SSL 证书（初始为自签名占位，后台可上传）
├── backup/                      升级备份
├── install.env                  安装参数（端口、数据库账号、密钥；权限 600）
└── install.sh                   部署脚本（sudo streamswitch 即运行它）
```
| systemd 服务 | 说明 |
|---|---|
| `streamswitch` | 后端（仅监听 127.0.0.1，由 Nginx 反代） |
| `streamswitch-zlm` | ZLMediaKit 流媒体内核 |

Nginx 站点：`/etc/nginx/conf.d/streamswitch.conf`；数据库：`streamswitch`（账号 `streamswitch`，密码随机生成）。

### 端口
| 端口 | 用途 | 是否对外 |
|---|---|---|
| 51880 / 51443 | 后台与播放（HTTP / HTTPS） | 需要外网访问时在路由器映射 |
| 1935 | RTMP 推流 / 播放 | 同上 |
| 554 | RTSP（可关闭） | 按需 |
| 18090 / 18080 | 后端 / ZLM 内部端口（被占用时自动顺延） | **不要**对外 |

若启用了 ufw，脚本会自动放行对外端口。

### 域名与证书
默认**不需要配置域名**：未添加播放域名时，播放与推流地址自动使用浏览器当前访问的地址（如 `http://192.168.1.10:51880`）。
需要正式域名时，登录后在「域名配置」中：上传 SSL 证书（支持直接选择 `fullchain.pem` / `privkey.pem` 文件）并「应用」，再添加推流/播放域名即可。

## 开发与构建

```
app/backend/        Go 后端（启动时自动建表/升级数据库：internal/database/schema.sql + seed_*.sql）
app/frontend/dist/  前端（原生 ES Modules，无需构建）
packaging/          打包脚本、ZLMediaKit 配置模板与版本、CI 安装测试
install.sh          一键部署脚本
tools/              运维诊断与测试脚本（读取 tools/.env.local，见 .env.example）
```

- **发布**：推送 `v*` 标签后，GitHub Actions 分别在 x86_64 与 ARM64 上编译 livems 与 ZLMediaKit 并打包，随后在全新的 Ubuntu 22.04 / 24.04（x86_64）与 Ubuntu 22.04（ARM64）上自动执行「安装 → 注册首个管理员 → 推流播放 → 重置密码 → 改端口 → 升级 → 卸载」全流程测试，全部通过才发布 Release。
- **本地打包**：
  ```bash
  (cd app/backend && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-s -w -X main.Version=1.1.0" -o /tmp/livems ./cmd/server)
  bash packaging/build-package.sh arm64 1.1.0 /tmp/livems /path/to/MediaServer dist
  ```
- **后端命令行**：`livems -migrate`（仅建表/升级）、`livems -list-users`、`livems -reset-password -user <用户名>`（密码经环境变量 `LIVEMS_NEW_PASSWORD` 传入）、`livems -version`。

## 改动记录

### v1.1.0 一键部署
- 中文菜单一键部署脚本：自动识别架构、安装依赖，安装 / 升级 / 端口设置 / 卸载 / 重置密码；无人值守参数；升级自动备份。
- 取消默认 `admin/admin123` 账号：首次打开后台强制注册，第一个用户为超级管理员（`/api/v1/setup`，命名锁防并发）。
- 后端启动时自动建表、补齐缺失列、写入内置角色权限，安装/升级不再需要手动导入 SQL。
- 未配置播放域名时用当前访问地址生成播放/推流地址（`X-Forwarded-Host`）。
- 双架构发布包与 CI 全流程安装测试。

### 前端
1. 表格「操作」列按钮统一单行水平排列；描述列表标签单行不换行。
2. 证书「删除」按钮使用中置灰并提示原因；证书上传支持选择文件（自动识别证书/私钥）。
3. 移动端适配（≤768px）：抽屉式侧边栏、窄屏工具栏/对话框/表单/分页。
4. 素材上传进度 100% 后显示落盘提示。

### 后端
1. 大文件素材上传改为流式写盘（660 MB：76s → 3.4s），审计中间件不再读取上传体，上传接口单独放宽超时。
2. 修复编辑制播任务报 `sql: expected 21 arguments, got 23`；编辑运行中任务后可正确重启。
3. 修复流「状态」开关禁用后推流不断开（`close_streams`）。
4. 修复无人观看时仪表盘观看人数非 0。
5. 接口错误提示中文化（数据库/网络底层报错自动转为中文，原文记日志）。
6. 可选 `server.pprof_addr` 诊断端口。

### 网络
- `tools/policy-route.sh`：主机默认网关是另一台代理网关（如 Surge 网关模式）时，修复路由器端口映射进来的连接回包走错路的问题（nftables 按入站 MAC 打标 + 策略路由），`--uninstall` 可回滚。

## 安全说明
- 数据库密码、JWT 与 ZLM 密钥在安装时随机生成，保存在 `/opt/streamswitch/install.env` 与配置文件中（权限 600）。
- 后端与 ZLM 内部端口仅供本机使用；对外请只开放 HTTP/HTTPS、RTMP（RTSP 按需）。
- 仓库不含任何站点私有信息；`tools/` 脚本的站点参数放在不入库的 `tools/.env.local`。

## License
原项目未声明许可证；本仓库仅作部署与改进记录。
