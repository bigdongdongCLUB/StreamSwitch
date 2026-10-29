#!/usr/bin/env python3
"""裸 TCP 吞吐测试 - 接收端（在 NanoPi 上运行，收一个连接后打印速率并退出）"""
import socket, sys, time

port = int(sys.argv[1]) if len(sys.argv) > 1 else 5202
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(("0.0.0.0", port))
s.listen(1)
s.settimeout(120)
c, addr = s.accept()
t = time.time()
n = 0
while True:
    b = c.recv(1 << 20)
    if not b:
        break
    n += len(b)
dt = time.time() - t
print("recv from %s: %.0f MB in %.2fs = %.1f MB/s (%.0f Mbps)" % (addr[0], n / 1e6, dt, n / 1e6 / dt, n * 8 / 1e6 / dt), flush=True)
