#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 真·外网连通性测试 + 抓包看回包走向（Mac 上运行）
#   用 check-host.net 的全球节点向公网 IP 发起 TCP 连接，同时在 NanoPi 上抓包，
#   观察外部 SYN 是否到达、SYN-ACK 回给了哪个 MAC（路由器 $ROUTER_MAC 还是 Mac $PROXY_GW_MAC）
set -uo pipefail
K=$SSH_KEY; H=$SSH_HOST
WAN=${WAN_IP:?.env.local 需设置 WAN_IP}
PORT=${1:-51880}
CTRL=${2:-8096}

ssh -i $K $H '. /root/livems-pkg/env.sh; command -v tcpdump >/dev/null || { export DEBIAN_FRONTEND=noninteractive; apt-get install -y -q tcpdump >/dev/null 2>&1; }; command -v tcpdump >/dev/null && echo "tcpdump ok" || echo "tcpdump missing"'

echo "### 在 NanoPi 上开始抓包（仅非内网来源，端口 ${PORT}，20 秒）"
ssh -i $K $H "nohup timeout 40 tcpdump -ni eth1 -e 'tcp port ${PORT} and not (src net $LAN_CIDR and dst net $LAN_CIDR)' > /root/cap.txt 2>&1 &"
sleep 1

chk() {  # check-host.net TCP 检测：返回各节点结果
  local target=$1
  local rid
  rid=$(curl -s -H 'Accept: application/json' "https://check-host.net/check-tcp?host=$target&max_nodes=3" | python3 -c 'import sys,json; print(json.load(sys.stdin).get("request_id",""))')
  [ -z "$rid" ] && { echo "    check-host.net 不可用"; return; }
  sleep 8
  curl -s -H 'Accept: application/json' "https://check-host.net/check-result/$rid" | python3 -c '
import sys,json
d=json.load(sys.stdin)
for node,res in d.items():
    if res is None: print("    %-28s 等待中" % node); continue
    r=res[0] if res else {}
    if isinstance(r,dict) and "time" in r: print("    %-28s 连通  %.0f ms  (对端 %s)" % (node, r["time"]*1000, r.get("address","")))
    elif isinstance(r,dict) and "error" in r: print("    %-28s 失败: %s" % (node, r["error"]))
    else: print("    %-28s %s" % (node, r))'
}

if [ "${CTRL}" != "0" ]; then echo "### 对照组：${WAN}:${CTRL}（emby，映射到 .116）"; chk "${WAN}:${CTRL}"; fi
echo "### 目标：${WAN}:${PORT}（StreamSwitch，映射到 .89）"
chk "${WAN}:${PORT}"

sleep 6
echo "### NanoPi 抓包结果（-e 显示链路层 MAC）"
ssh -i $K $H '. /root/livems-pkg/env.sh; cat /root/cap.txt | grep -vE "^(tcpdump|listening|[0-9]+ packets)" | head -n 20 | sed "s/^/    /"; echo; grep -E "packets (captured|received)" /root/cap.txt | sed "s/^/    /"'
echo "    路由器 MAC = $ROUTER_MAC   代理网关 = $PROXY_GW_IP"
echo "### TEST_DONE"
