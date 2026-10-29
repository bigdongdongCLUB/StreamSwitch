# 迁移指南（宿主机 → 宿主机，同为宝塔环境）

适用于：把整个流媒体系统从当前服务器完整迁移到另一台**同样装了宝塔面板**的 x86_64 服务器。

> 你的业务依赖本机网络环境（转推 YouTube/ins），所以方案是纯宿主机整体搬迁，
> 不涉及任何云服务。Docker 化暂不支持（ffmpeg 制播 + 宝塔 Nginx 耦合，容器化收益低）。

## 架构与迁移内容

```
迁移包（livems-migrate-<时间戳>.tar.gz）
├── app/                     应用目录（/www/wwwroot/live.rextian.com）
│   ├── backend/             Go 后端二进制 + ffmpeg 工具 + 中文字体 + 配置 + 素材
│   ├── frontend/dist/       前端产物
│   ├── deploy/  sql/  docs/ 部署模板与数据库结构
├── db/live_rextian_com.sql  MySQL 全库 dump（含全部业务数据/权限/配置）
├── zlm/                     ZLMediaKit 二进制 + config.ini
├── systemd/                 livems.service + zlm.service
├── certs/                   SSL 证书（宝塔目录约定）
└── MIGRATION_INFO.txt       迁移清单（版本/架构/行数校验）
```

**不需要迁移**：Redis 数据（播放令牌有 MySQL 留档兜底，重启后自动重建）、
历史操作日志与流量采样（无业务价值）、ZLM 的 HLS 切片缓存（实时生成）。

## 操作步骤

### 1. 源服务器打包

```bash
cd /www/wwwroot/live.rextian.com
bash deploy/migrate/pack.sh                  # 完整打包（含素材库）
# 素材很多想分开传时：bash deploy/migrate/pack.sh --no-materials
```

产物：`/www/wwwroot/livems-migrate-<时间戳>.tar.gz`

### 2. 传输到新服务器

```bash
scp /www/wwwroot/livems-migrate-*.tar.gz root@<新服务器IP>:/root/
```

### 3. 新服务器准备（宝塔）

- 装好 **Nginx / MySQL / Redis**（宝塔软件商店）
- 宝塔里**创建一个站点**（域名随意，只为让宝塔生成 vhost 目录结构）
- 确认新服务器架构为 x86_64（`uname -m`）

### 4. 还原

```bash
cd /root && mkdir livems-migrate && tar -xzf livems-migrate-*.tar.gz -C /tmp/  # 或直接：
bash <(tar -xzOf livems-migrate-*.tar.gz ./deploy/migrate/restore.sh) /root/livems-migrate-*.tar.gz
# 或者先把包解开再执行：
mkdir -p /tmp/livems-pkg && tar -xzf /root/livems-migrate-*.tar.gz -C /tmp/livems-pkg
bash /tmp/livems-pkg/app/deploy/migrate/restore.sh /root/livems-migrate-*.tar.gz
```

按提示输入新服务器 MySQL root 密码（用于建库建账号）。

### 5. 迁移后检查

| 检查项 | 命令/方法 |
|---|---|
| 服务状态 | `systemctl status zlm livems` |
| API 健康 | `curl http://127.0.0.1:8090/health` |
| 数据行数 | 对比包内 `MIGRATION_INFO.txt` |
| 后台登录 | `http://<IP>:51880/`（原账号密码） |
| 推流测试 | OBS 推一路，看「流管理」在线 |
| 播放测试 | 「播放地址」生成 → VLC 播放 |
| 制播测试 | 轮播任务能拉起 ffmpeg |
| DNS 切换 | A 记录指向新 IP；旧机停服 |

## 域名/IP 变更时的收尾

新服务器公网 IP 不同的话，迁移后只需在后台改两处：

1. **「域名配置」**：推流域名/播放域名的主机与端口（如果域名不变，只需改 DNS A 记录指向新 IP，后台不用动）
2. **证书**：域名不变且已还原证书 → 直接用；换了域名 → 宝塔重新申请

## 与「宝塔备份」的关系

宝塔自带的网站备份只能覆盖**网站目录 + 数据库**，缺 ZLM、Nginx conf、systemd、证书、
ffmpeg 工具和中文字体。本方案把这些全部打进去，比宝塔备份完整。
如果你坚持用宝塔备份：网站备份 + 数据库备份之外，还需手动再备份这三样——
`/www/server/zlm/`、`/etc/systemd/system/{livems,zlm}.service`、
`/www/server/panel/vhost/{nginx/live.rextian.com.conf, cert/live.rextian.com/}`。
