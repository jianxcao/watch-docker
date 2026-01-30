#!/bin/bash
# Watch Docker 包卸载前脚本

set -e

# 停止所有运行中的服务实例
for service in $(systemctl list-units --type=service --state=running | grep 'watch-docker@' | awk '{print $1}'); do
    echo "停止服务: $service"
    systemctl stop "$service" || true
    systemctl disable "$service" || true
done

# 重载 systemd
systemctl daemon-reload || true

echo ""
echo "Watch Docker 服务已停止"
echo ""
echo "注意：配置文件保留在 ~/.watch-docker"
echo "如需完全删除，请手动删除该目录"
echo ""

exit 0
