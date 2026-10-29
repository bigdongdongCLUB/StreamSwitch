#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# =============================================================================
#  NanoPi 策略路由：修复「默认网关是 Mac(Surge) 导致路由器端口映射进来的连接回包走错」
#
#  原理：
#    1. nftables 在 PREROUTING 给「源 MAC = 路由器」的入站连接打 conntrack 标记 0x1，
#       回包（OUTPUT）通过 ct mark 继承该标记；
#    2. ip rule：fwmark 0x1 → 查 viarouter 路由表（默认网关 = 路由器 $ROUTER_IP）；
#    3. 其余出站流量仍走主表（默认网关 = 代理网关 $PROXY_GW_IP，保留代理出口）。
#  参数全部来自同目录 .env.local（IFACE / LAN_CIDR / ROUTER_IP / ROUTER_MAC / PROXY_GW_IP）。
#
#  持久化：nftables.service 加载 /etc/nftables.conf；NetworkManager dispatcher 在 eth1 up 时重建路由表与规则。
#  回滚：bash policy-route.sh --uninstall
# =============================================================================
set -euo pipefail

# 以下来自 .env.local：IFACE / ROUTER_IP / ROUTER_MAC / LAN_CIDR
: "${ROUTER_IP:?.env.local 需设置 ROUTER_IP}"
: "${ROUTER_MAC:?.env.local 需设置 ROUTER_MAC（ip neigh 可查）}"
MARK=0x1
TABLE_ID=100
TABLE_NAME=viarouter
NFT_FILE=/etc/nftables.d/asym-fix.nft
DISPATCH=/etc/NetworkManager/dispatcher.d/50-policy-route
RT_TABLES=/etc/iproute2/rt_tables.d/policy-route.conf

if [[ "${1:-}" == "--uninstall" ]]; then
  echo "==> 卸载策略路由"
  rm -f "$NFT_FILE" "$DISPATCH" "$RT_TABLES"
  sed -i '\#include "/etc/nftables.d/\*.nft"#d' /etc/nftables.conf 2>/dev/null || true
  nft delete table inet asymfix 2>/dev/null || true
  ip rule del fwmark $MARK table $TABLE_ID 2>/dev/null || true
  ip route flush table $TABLE_ID 2>/dev/null || true
  echo "    已恢复为单网关（Mac）模式"
  exit 0
fi

echo "==> [1/5] 安装 nftables"
export DEBIAN_FRONTEND=noninteractive
command -v nft >/dev/null || apt-get install -y -q nftables >/dev/null
systemctl enable nftables >/dev/null 2>&1 || true

echo "==> [2/5] 校验路由器 MAC（当前 ARP 表）"
CUR_MAC=$(ip neigh show "$ROUTER_IP" dev "$IFACE" | awk '{print $3}' | head -1)
if [[ -z "$CUR_MAC" ]]; then ping -c1 -W1 "$ROUTER_IP" >/dev/null 2>&1 || true; CUR_MAC=$(ip neigh show "$ROUTER_IP" dev "$IFACE" | awk '{print $3}' | head -1); fi
if [[ -n "$CUR_MAC" && "$CUR_MAC" != "$ROUTER_MAC" ]]; then
  echo "    警告: ARP 中 $ROUTER_IP 的 MAC 为 $CUR_MAC，与 .env.local 的 $ROUTER_MAC 不同，改用 ARP 实测值"
  ROUTER_MAC=$CUR_MAC
fi
echo "    路由器 $ROUTER_IP = $ROUTER_MAC"

echo "==> [3/5] 写入 nftables 规则"
mkdir -p /etc/nftables.d
cat > "$NFT_FILE" <<EOF
# 由 policy-route.sh 生成：给来自路由器的入站连接打标，使回包经路由器返回
table inet asymfix {
    chain prerouting {
        type filter hook prerouting priority mangle; policy accept;
        # 新连接且来自路由器 MAC（即公网经端口映射进来）→ 连接打标
        iifname "$IFACE" ether saddr $ROUTER_MAC ct state new ct mark set $MARK
    }
    chain output {
        type route hook output priority mangle; policy accept;
        # 本机发出的回包继承连接标记 → 命中 ip rule 走 viarouter 表
        ct mark $MARK meta mark set ct mark
    }
}
EOF
grep -q 'include "/etc/nftables.d/\*.nft"' /etc/nftables.conf || echo 'include "/etc/nftables.d/*.nft"' >> /etc/nftables.conf
nft -c -f /etc/nftables.conf
nft delete table inet asymfix 2>/dev/null || true
nft -f "$NFT_FILE"
systemctl restart nftables

echo "==> [4/5] 路由表与策略规则（含开机自启）"
mkdir -p /etc/iproute2/rt_tables.d
echo "$TABLE_ID $TABLE_NAME" > "$RT_TABLES"
cat > "$DISPATCH" <<EOF
#!/bin/bash
# 由 policy-route.sh 生成：$IFACE 启动后重建 viarouter 路由表与 fwmark 规则
[ "\$1" = "$IFACE" ] || exit 0
case "\$2" in up|dhcp4-change|connectivity-change) ;; *) exit 0 ;; esac
ip route replace default via $ROUTER_IP dev $IFACE table $TABLE_ID
ip route replace $LAN_CIDR dev $IFACE table $TABLE_ID
ip rule del fwmark $MARK table $TABLE_ID 2>/dev/null
ip rule add fwmark $MARK table $TABLE_ID priority 100
# 非对称路径下保持宽松反向路径校验
sysctl -qw net.ipv4.conf.$IFACE.rp_filter=2 net.ipv4.conf.all.rp_filter=2
EOF
chmod +x "$DISPATCH"
"$DISPATCH" "$IFACE" up

echo "==> [5/5] 当前生效状态"
echo "--- nft:"; nft list table inet asymfix | sed 's/^/    /'
echo "--- ip rule:"; ip rule show | sed 's/^/    /'
echo "--- table $TABLE_NAME:"; ip route show table $TABLE_ID | sed 's/^/    /'
echo "--- 主表默认路由（应仍为 Mac）:"; ip route show default | sed 's/^/    /'
echo "--- 出口公网 IP（应仍为代理出口）:"; curl -4 -s --max-time 6 https://api.ipify.org; echo
echo "==> 完成"
