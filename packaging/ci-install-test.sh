#!/usr/bin/env bash
# CI：在全新 Ubuntu 上无人值守执行 安装 → 首个管理员注册 → 登录 → 重置密码 → 改端口 → 升级 → 卸载
set -euo pipefail
PKG=${1:?安装包目录}
export SS_YES=1
H=http://127.0.0.1
# 端口可覆盖（在已有其他服务的主机上测试时避开冲突）
P1=${TEST_HTTP:-51880}; S1=${TEST_HTTPS:-51443}; R1=${TEST_RTMP:-1935}; T1=${TEST_RTSP:-554}
P2=${TEST_HTTP2:-52880}; S2=${TEST_HTTPS2:-52443}; R2=${TEST_RTMP2:-2935}
export SS_HTTP_PORT=$P1 SS_HTTPS_PORT=$S1 SS_RTMP_PORT=$R1 SS_RTSP_PORT=$T1
j() { python3 -c "import sys,json; d=json.load(sys.stdin); print($1)"; }
step() { echo; echo "::group::$*"; }
end() { echo "::endgroup::"; }

step "1. 安装"
test ! -d /opt/streamswitch   # 必须是全新主机
sudo -E bash "$PKG/install.sh" install | tee /tmp/ss-install.log
grep -q '全新安装' /tmp/ss-install.log
grep -q '部署完成' /tmp/ss-install.log
test "$(cat /opt/streamswitch/VERSION)" = "$(cat "$PKG/VERSION")"
systemctl is-active streamswitch streamswitch-zlm nginx
end

step "2. 未初始化：必须先注册首个超级管理员"
curl -fsS $H:$P1/api/v1/setup/status | tee /dev/stderr | grep -q '"need_setup":true'
curl -fsS -X POST $H:$P1/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin123"}' | grep -q '尚未初始化'
curl -sS -X POST $H:$P1/api/v1/setup -H 'Content-Type: application/json' -d '{"username":"boss","password":"short"}' | grep -q '不能少于 8 位'
TOKEN=$(curl -fsS -X POST $H:$P1/api/v1/setup -H 'Content-Type: application/json' -d '{"username":"boss","password":"Passw0rd!x","nickname":"老板"}' | j 'd["data"]["token"]')
test -n "$TOKEN"
curl -fsS -H "Authorization: Bearer $TOKEN" $H:$P1/api/v1/auth/profile | j 'd["data"]["is_super"]' | grep -qx 1
code=$(curl -sS -o /dev/null -w '%{http_code}' -X POST $H:$P1/api/v1/setup -H 'Content-Type: application/json' -d '{"username":"evil","password":"Passw0rd!x"}')
test "$code" = 400   # 已初始化后不允许再注册
end

step "3. 域名留空时播放/推流地址使用访问地址"
SID=$(curl -fsS -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' $H:$P1/api/v1/streams -d '{"name":"ci","app":"live","stream":"ci01"}' | j 'd["data"]["stream"]["id"]')
curl -fsS -H "Authorization: Bearer $TOKEN" -H "Host: 10.9.8.7:$P1" $H:$P1/api/v1/streams/$SID/urls | tee /dev/stderr | grep -q "http://10.9.8.7:$P1/live/ci01.live.flv"
curl -fsS -H "Authorization: Bearer $TOKEN" -H "Host: 10.9.8.7:$P1" $H:$P1/api/v1/streams/$SID/info | grep -q "rtmp://10.9.8.7$( [ $R1 = 1935 ] || echo :$R1 )/live/ci01"
end

step "4. 推流 + HTTP-FLV 播放"
KEY=$(curl -fsS -H "Authorization: Bearer $TOKEN" $H:$P1/api/v1/streams/$SID/info | j 'd["data"]["push_key"]')
timeout 25 ffmpeg -hide_banner -loglevel error -re -f lavfi -i testsrc=size=320x240:rate=25 -f lavfi -i sine -c:v libx264 -preset ultrafast -g 25 -c:a aac -f flv "rtmp://127.0.0.1:$R1/live/ci01?key=$KEY" &
sleep 8
PT=$(curl -fsS -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' $H:$P1/api/v1/tokens -d "{\"stream_id\":$SID,\"expire_sec\":600}" | j 'd["data"]["token"]')
n=$(curl -s --max-time 4 -o /tmp/ci.flv -w '%{size_download}' "$H:$P1/live/ci01.live.flv?token=$PT" || true)
echo "flv bytes: $n"; test "${n:-0}" -gt 10000
wait || true
end

step "5. 重置密码（命令行菜单 4）"
SS_RESET_USER=boss SS_RESET_PASSWORD=NewPassw0rd sudo -E bash /opt/streamswitch/install.sh reset-password
curl -fsS -X POST $H:$P1/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"boss","password":"NewPassw0rd"}' | grep -q '"token"'
end

step "6. 端口设置（菜单 2）"
SS_HTTP_PORT=$P2 SS_HTTPS_PORT=$S2 SS_RTMP_PORT=$R2 SS_RTSP_PORT=0 sudo -E bash /opt/streamswitch/install.sh port
curl -fsS $H:$P2/health | grep -q '"code":0'
curl -fsSk https://127.0.0.1:$S2/health | grep -q '"code":0'
ss -ltn | grep -q ":$R2 "
for i in $(seq 1 20); do ss -ltn | grep -q ":$P1 " || break; sleep 0.5; done
if ss -ltn | grep -q ":$P1 "; then echo "旧端口 $P1 仍在监听"; exit 1; fi
end

step "7. 升级（再次执行安装）"
SS_FORCE=1 sudo -E bash "$PKG/install.sh" install | tee /tmp/ss-upgrade.log
grep -q '执行升级' /tmp/ss-upgrade.log
ls -d /opt/streamswitch/backup/*/ | head -1
curl -fsS -X POST $H:$P2/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"boss","password":"NewPassw0rd"}' | grep -q '"token"'
curl -fsS -H "Authorization: Bearer $TOKEN" $H:$P2/api/v1/streams | grep -q 'ci01'
end

step "8. 卸载（菜单 3，含删除数据）"
SS_PURGE=1 sudo -E bash /opt/streamswitch/install.sh uninstall
test ! -d /opt/streamswitch
if systemctl list-unit-files | grep -q '^streamswitch'; then echo "systemd 单元未清理"; exit 1; fi
test ! -f /etc/nginx/conf.d/streamswitch.conf
if mysql -uroot -e 'SHOW DATABASES' 2>/dev/null | grep -qx streamswitch || MYSQL_PWD=${SS_MYSQL_ROOT_PASSWORD:-x} mysql -uroot -h127.0.0.1 -e 'SHOW DATABASES' 2>/dev/null | grep -qx streamswitch; then echo "数据库未删除"; exit 1; fi
end
echo "全部通过"
