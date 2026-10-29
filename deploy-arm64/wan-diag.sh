#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 外网访问不通诊断（在 NanoPi 上执行）
echo "### 1. 路由表 / 网关"
ip route show
echo "--- 网关与 .1 的 MAC（判断网关是不是路由器本身）:"
ip neigh show | grep -E "($ROUTER_IP|$PROXY_GW_IP) " || arp -n | grep -E "($ROUTER_IP|$PROXY_GW_IP) "
echo "--- netplan 配置:"
cat /etc/netplan/*.yaml 2>/dev/null | grep -vE '^\s*#' | sed 's/^/    /'
echo
echo "### 2. 出口公网 IP（对比路由器 WAN $WAN_IP）"
for u in https://api.ipify.org https://ifconfig.me/ip https://4.ipw.cn; do
  r=$(curl -4 -s --max-time 6 "$u"); [ -n "$r" ] && { echo "    $u -> $r"; break; }
done
echo "--- 到 8.8.8.8 的前 3 跳:"
traceroute -n -m 3 -w 1 8.8.8.8 2>/dev/null | tail -n +2 | sed 's/^/    /' || tracepath -n -m 3 8.8.8.8 2>/dev/null | head -4
echo
echo "### 3. 防火墙"
ufw status 2>/dev/null | head -5
echo "--- nft/iptables 规则数:"; nft list ruleset 2>/dev/null | grep -c . ; iptables -S 2>/dev/null | grep -vE '^-P' | head -5
echo
echo "### 4. 监听（必须是 0.0.0.0 / *）"
ss -ltn | grep -E ':(51880|51443|1935|554) '
echo
echo "### 5. Nginx 对任意 Host 的响应"
curl -s -o /dev/null -w "    Host=$DDNS_DOMAIN :51880 -> HTTP %{http_code}\n" -H 'Host: $DDNS_DOMAIN' http://127.0.0.1:51880/health
curl -sk -o /dev/null -w "    Host=$DDNS_DOMAIN :51443 -> HTTP %{http_code}\n" --resolve $DDNS_DOMAIN:51443:127.0.0.1 https://$DDNS_DOMAIN:51443/health
echo
echo "### 6. DDNS 解析"
getent hosts $DDNS_DOMAIN | sed 's/^/    /'
