#!/bin/bash
# Watch Docker 包安装后脚本

set -e

# 检测当前用户
CURRENT_USER="${SUDO_USER:-${USER}}"
HOME_DIR=$(eval echo "~$CURRENT_USER")
CONFIG_DIR="$HOME_DIR/.watch-docker"

# 创建配置目录
if [ ! -d "$CONFIG_DIR" ]; then
    mkdir -p "$CONFIG_DIR"
    chown "$CURRENT_USER:$CURRENT_USER" "$CONFIG_DIR"
    chmod 755 "$CONFIG_DIR"
fi

# 复制配置文件示例（如果存在）
# 应用配置（app.yaml）
APP_EXAMPLE_SOURCE="app.yaml.example"
if [ -f "$APP_EXAMPLE_SOURCE" ]; then
    APP_EXAMPLE_TARGET="$CONFIG_DIR/app.yaml.example"
    cp "$APP_EXAMPLE_SOURCE" "$APP_EXAMPLE_TARGET"
    chown "$CURRENT_USER:$CURRENT_USER" "$APP_EXAMPLE_TARGET"
    echo "✅ 已复制应用配置示例: $APP_EXAMPLE_TARGET"
    
    # 如果用户应用配置不存在，创建默认配置
    USER_APP_CONFIG="$CONFIG_DIR/app.yaml"
    if [ ! -f "$USER_APP_CONFIG" ]; then
        cp "$APP_EXAMPLE_SOURCE" "$USER_APP_CONFIG"
        chown "$CURRENT_USER:$CURRENT_USER" "$USER_APP_CONFIG"
        chmod 600 "$USER_APP_CONFIG"
        echo "✅ 已创建默认应用配置: $USER_APP_CONFIG"
        echo "⚠️  请编辑 app.yaml 并修改默认密码！"
    fi
fi

# Docker 业务配置（config.yaml）
CONFIG_EXAMPLE_SOURCE="config.yaml.example"
if [ -f "$CONFIG_EXAMPLE_SOURCE" ]; then
    CONFIG_EXAMPLE_TARGET="$CONFIG_DIR/config.yaml.example"
    cp "$CONFIG_EXAMPLE_SOURCE" "$CONFIG_EXAMPLE_TARGET"
    chown "$CURRENT_USER:$CURRENT_USER" "$CONFIG_EXAMPLE_TARGET"
    echo "✅ 已复制业务配置示例: $CONFIG_EXAMPLE_TARGET"
    
    # 如果用户业务配置不存在，创建默认配置
    USER_CONFIG="$CONFIG_DIR/config.yaml"
    if [ ! -f "$USER_CONFIG" ]; then
        cp "$CONFIG_EXAMPLE_SOURCE" "$USER_CONFIG"
        chown "$CURRENT_USER:$CURRENT_USER" "$USER_CONFIG"
        chmod 600 "$USER_CONFIG"
        echo "✅ 已创建默认业务配置: $USER_CONFIG"
    fi
fi

echo ""
echo "================================================"
echo "Watch Docker 安装完成！"
echo "================================================"
echo ""
echo "配置目录: $CONFIG_DIR"
echo ""
echo "配置文件："
echo "  应用配置: $CONFIG_DIR/app.yaml        (用户名、密码、功能开关)"
echo "  业务配置: $CONFIG_DIR/config.yaml    (Docker 扫描、通知等)"
echo ""
echo "配置示例："
echo "  $CONFIG_DIR/app.yaml.example"
echo "  $CONFIG_DIR/config.yaml.example"
echo ""
echo "启动方式（选择其一）："
echo ""
echo "方式 1: 使用标准服务（推荐）"
echo "  sudo systemctl enable watch-docker"
echo "  sudo systemctl start watch-docker"
echo ""
echo "方式 2: 使用用户模板服务"
echo "  sudo systemctl enable watch-docker@$CURRENT_USER"
echo "  sudo systemctl start watch-docker@$CURRENT_USER"
echo ""
echo "方式 3: 直接运行"
echo "  watch-docker"
echo ""
echo "访问地址: http://localhost:8080"
echo "默认账户: admin / admin"
echo ""
echo "⚠️  安全提示："
echo "  1. 请修改应用配置中的默认密码"
echo "  2. 编辑 $CONFIG_DIR/app.yaml"
echo "  3. 修改后重启服务"
echo ""
echo "📝 配置说明："
echo "  - app.yaml    应用配置（用户名、密码、2FA 等）"
echo "  - config.yaml 业务配置（扫描、通知、服务器等）"
echo ""
echo "================================================"
echo ""

exit 0
