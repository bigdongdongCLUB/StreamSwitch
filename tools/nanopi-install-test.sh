#!/usr/bin/env bash
# 在已有旧部署的主机上（用不冲突的端口）跑一遍一键部署全流程测试。Mac 上运行：
#   bash tools/nanopi-install-test.sh <streamswitch-linux-<arch>.tar.gz>
. "$(dirname "$0")/env.sh"
set -uo pipefail
PKG=${1:?安装包路径}
scp -q -i "$SSH_KEY" "$PKG" "$(dirname "$0")/../packaging/ci-install-test.sh" "$SSH_HOST":/root/
ssh -i "$SSH_KEY" "$SSH_HOST" 'bash -s' <<'REMOTE'
set -uo pipefail
rm -rf /root/sstest && mkdir -p /root/sstest && tar -xzf /root/streamswitch-linux-*.tar.gz -C /root/sstest
# 用新包里的脚本卸载上一次的测试安装，并确认清理干净（保证本次走全新安装路径）
if [ -d /opt/streamswitch ] || [ -f /etc/systemd/system/streamswitch.service ]; then
  SS_YES=1 SS_PURGE=1 bash /root/sstest/streamswitch/install.sh uninstall 2>&1 | tail -n 3
fi
if [ -d /opt/streamswitch ]; then echo "!! 旧测试安装未清理干净"; exit 1; fi
cd /root
TEST_HTTP=53880 TEST_HTTPS=53443 TEST_RTMP=3935 TEST_RTSP=0 TEST_HTTP2=54880 TEST_HTTPS2=54443 TEST_RTMP2=4935 \
  bash /root/ci-install-test.sh /root/sstest/streamswitch > /root/sstest.log 2>&1
rc=$?
echo "TEST rc=$rc"
grep -E "^::group::|全部通过|✘|旧端口|未清理|未删除|flv bytes" /root/sstest.log
echo "---- 日志尾部 ----"
tail -n 30 /root/sstest.log
REMOTE
