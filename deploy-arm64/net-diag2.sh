#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# NanoPi 侧：链路事件 / 中断分布 / 软中断丢包
echo "--- dmesg (r8125/eth1 最近事件):"
dmesg 2>/dev/null | grep -iE "r8125|eth1" | tail -n 8
echo "--- eth1 中断分布 (前4行):"
grep -E "eth1|r8125" /proc/interrupts | head -4
echo "--- softnet 丢包合计:"
awk '{ d += strtonum("0x" $2) } END { print "  softnet dropped:", d }' /proc/net/softnet_stat
echo "--- CPU 频率 (MHz, 各核):"
for f in /sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq; do printf '%s ' $(( $(cat $f) / 1000 )); done; echo
echo "--- 调速器:"; cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor 2>/dev/null
