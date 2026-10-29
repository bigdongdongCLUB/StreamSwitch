#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# NanoPi 网口协商速率与链路诊断
echo "### 接口与地址"
ip -br link; echo; ip -br addr | grep -v '^lo'
echo
echo "### 默认路由（$HOST_IP 走哪个口）"
ip route get $ROUTER_IP 2>/dev/null | head -1; ip route show default
echo
echo "### 每个物理口的协商结果（ethtool）"
for i in $(ls /sys/class/net | grep -vE '^(lo|docker|veth|br-|wlan)'); do
  echo "--- $i  (driver: $(basename "$(readlink /sys/class/net/$i/device/driver 2>/dev/null)" 2>/dev/null))  carrier=$(cat /sys/class/net/$i/carrier 2>/dev/null)  sysfs_speed=$(cat /sys/class/net/$i/speed 2>/dev/null)Mb/s duplex=$(cat /sys/class/net/$i/duplex 2>/dev/null)"
  ethtool "$i" 2>/dev/null | grep -E 'Supported link modes|Advertised link modes|Link partner advertised link modes|Speed|Duplex|Auto-negotiation|Link detected' | sed 's/^/    /'
done
echo
echo "### 无线口（如有）"
for w in $(ls /sys/class/net | grep -E '^wl'); do iw dev "$w" link 2>/dev/null | sed "s/^/    [$w] /"; done
echo
echo "### 主用口的错误/丢包计数"
MAIN=$(ip route show default | awk '{print $5}' | head -1)
ip -s link show "$MAIN" | sed 's/^/    /'
ethtool -S "$MAIN" 2>/dev/null | grep -iE 'err|drop|crc|collision|pause' | grep -vE ': 0$' | sed 's/^/    /' | head -20
echo
echo "### 网口 offload / 队列"
ethtool -k "$MAIN" 2>/dev/null | grep -E '^(rx-checksumming|tx-checksumming|scatter-gather|tcp-segmentation-offload|generic-receive-offload|generic-segmentation-offload):' | sed 's/^/    /'
echo "    txqueuelen=$(cat /sys/class/net/$MAIN/tx_queue_len) mtu=$(cat /sys/class/net/$MAIN/mtu)"
echo
echo "### 内核网络参数"
sysctl net.core.rmem_max net.core.wmem_max net.ipv4.tcp_rmem net.ipv4.tcp_wmem net.ipv4.tcp_congestion_control 2>/dev/null | sed 's/^/    /'
echo
echo "### iperf3 可用性"
command -v iperf3 >/dev/null && echo "    iperf3 已安装: $(iperf3 --version | head -1)" || echo "    iperf3 未安装"
