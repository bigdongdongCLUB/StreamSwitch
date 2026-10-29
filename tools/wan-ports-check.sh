#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 用 check-host.net 海外节点逐个检查公网端口连通性（Mac 上运行）
set -uo pipefail
WAN=${WAN:-${WAN_IP:?.env.local 需设置 WAN_IP}}
PORTS=${*:-51880 51443 1935 554}
for p in $PORTS; do
  rid=$(curl -s -H 'Accept: application/json' "https://check-host.net/check-tcp?host=${WAN}:${p}&max_nodes=3" | python3 -c 'import sys,json; print(json.load(sys.stdin).get("request_id",""))')
  [ -z "$rid" ] && { echo "  :$p  check-host.net 不可用"; continue; }
  sleep 7
  curl -s -H 'Accept: application/json' "https://check-host.net/check-result/$rid" | python3 -c "
import sys,json
d=json.load(sys.stdin); ok=0; tot=0; errs=[]
for node,res in d.items():
    tot+=1; r=(res or [{}])[0] if res else {}
    if isinstance(r,dict) and 'time' in r: ok+=1
    elif isinstance(r,dict) and 'error' in r: errs.append(r['error'])
print('  :%-5s  %d/%d 节点连通%s' % ('$p', ok, tot, ('  失败: '+', '.join(set(errs))) if errs else ''))"
done
