#!/usr/bin/env bash
# 删除测试主机上的一键部署测试实例（含数据），并确认旧部署仍正常。Mac 上运行。
. "$(dirname "$0")/env.sh"
TMP=$(mktemp "${TMPDIR:-/tmp}/sscleanup.XXXXXX")
cat > "$TMP" <<'REMOTE'
if [ -x /opt/streamswitch/install.sh ]; then SS_YES=1 SS_PURGE=1 bash /opt/streamswitch/install.sh uninstall 2>&1 | grep -E '✔|✘|!'; fi
rm -rf /root/sstest /root/streamswitch-linux-*.tar.gz /root/ci-install-test.sh /root/sstest.log /root/menu-test* /root/menu-result.txt
echo "--- 残留检查"
ls -d /opt/streamswitch* 2>/dev/null || echo "  /opt/streamswitch 已清理"
systemctl list-unit-files 'streamswitch*' --no-legend | grep . || echo "  systemd 单元已清理"
mysql -uroot -N -e "SHOW DATABASES LIKE 'streamswitch'" | grep . || echo "  数据库已清理"
echo "--- 旧部署"
for s in livems zlm nginx mysql redis-server; do printf '  %-13s %s\n' $s "$(systemctl is-active $s)"; done
curl -s -o /dev/null -w '  旧后台 :51880 -> HTTP %{http_code}\n' http://127.0.0.1:51880/health
REMOTE
scp -q -i "$SSH_KEY" "$TMP" "$SSH_HOST":/root/sscleanup.sh && rm -f "$TMP"
ssh -i "$SSH_KEY" "$SSH_HOST" 'bash /root/sscleanup.sh </dev/null; rm -f /root/sscleanup.sh'
