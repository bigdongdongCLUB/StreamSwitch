#!/usr/bin/env bash
# 在主机终端里演示 install.sh 的进度显示（需在真实终端中运行：ssh -t）
#   1) 清空 apt 缓存后仅下载若干大包（显示下载进度）  2) 安装并卸载一个小包（显示安装进度）
# 用法：bash progress-demo.sh /path/to/install.sh
set -Eeo pipefail
head -n -1 "${1:?install.sh 路径}" > /tmp/ss-lib.sh   # 去掉最后一行 main "$@"
. /tmp/ss-lib.sh
banner
apt-get clean
info "演示 1：下载软件包"
run_progress "下载完成" env DEBIAN_FRONTEND=noninteractive apt-get install -y -d --reinstall "${APT_OPTS[@]}" \
  libavcodec60 libavfilter9 libavformat60 mysql-server-core-8.0 libx265-199 || die "下载失败"
info "演示 2：安装小软件包"
apt_install cowsay sl
run_progress "已移除演示软件包" env DEBIAN_FRONTEND=noninteractive apt-get remove -y "${APT_OPTS[@]}" cowsay sl
info "演示 3：无百分比的长任务"
run_progress "等待完成" sleep 6
echo "DEMO_DONE"
