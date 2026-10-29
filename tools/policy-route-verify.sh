#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 策略路由生效后的综合验证（Mac 上运行）
set -uo pipefail
K=$SSH_KEY; H=$SSH_HOST
echo "### 1. 公网端口连通性（海外节点）"
bash "$(dirname "$0")/wan-ports-check.sh" 51443 1935 554
echo
echo "### 2. NanoPi 代理出口 / 服务 / 持久化"
ssh -i $K $H '. /root/livems-pkg/env.sh; 
echo "  出口 IP: $(curl -4 -s --max-time 6 https://api.ipify.org)   (代理出口应 ≠ 路由器公网 IP $WAN_IP)"
echo "  github.com: HTTP $(curl -s -o /dev/null --max-time 8 -w %{http_code} https://github.com)"
echo "  服务: $(systemctl is-active $SVC_ZLM $SVC_APP nginx | tr "\n" " ")"
echo "  nftables 开机自启: $(systemctl is-enabled nftables 2>/dev/null)"
echo "  持久化文件:"; ls -la /etc/NetworkManager/dispatcher.d/50-policy-route /etc/nftables.d/asym-fix.nft /etc/iproute2/rt_tables.d/policy-route.conf | awk "{print \"    \" \$1, \$NF}"
echo "  nft 计数（prerouting 打标命中次数）:"; nft -a list chain inet asymfix prerouting 2>/dev/null | grep -oE "counter.*" || echo "    (规则无计数器，略)"
'
echo "### VERIFY_DONE"
