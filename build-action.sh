#!/bin/bash

# 请使用 release.sh 脚本代替此脚本
# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 默认参数
TAG="test"
PUSH="false"
SECRETS_FILE=".secrets"

# 显示帮助信息
show_help() {
    cat << EOF
使用方法: $0 [选项]

选项:
    -t, --tag TAG           Docker 镜像标签 (默认: test)
    -p, --push              是否推送到 Docker Hub (默认: 不推送)
    -s, --secrets FILE      secrets 文件路径 (默认: .secrets)
    -h, --help              显示此帮助信息

示例:
    $0                      # 构建 test 标签，不推送
    $0 -t latest            # 构建 latest 标签，不推送
    $0 -t v1.0.0 -p         # 构建 v1.0.0 标签并推送
    $0 -t latest -p -s .secrets.prod  # 使用自定义 secrets 文件

EOF
}

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -t|--tag)
            TAG="$2"
            shift 2
            ;;
        -p|--push)
            PUSH="true"
            shift
            ;;
        -s|--secrets)
            SECRETS_FILE="$2"
            shift 2
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            echo -e "${RED}❌ 未知参数: $1${NC}"
            show_help
            exit 1
            ;;
    esac
done

# 打印配置信息
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}🐳 本地构建 Docker 镜像${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}📋 构建配置:${NC}"
echo -e "  • 标签 (tag):        ${YELLOW}${TAG}${NC}"
echo -e "  • 推送 (push):       ${YELLOW}${PUSH}${NC}"
echo -e "  • Secrets 文件:      ${YELLOW}${SECRETS_FILE}${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# 检查 act 是否安装
echo -e "${GREEN}🔍 检查依赖...${NC}"
if ! command -v act &> /dev/null; then
    echo -e "${RED}❌ 错误: act 未安装${NC}"
    echo -e "${YELLOW}请运行以下命令安装:${NC}"
    echo -e "  brew install act"
    exit 1
fi
echo -e "${GREEN}✅ act 已安装: $(act --version)${NC}"

# 检查 secrets 文件
if [ ! -f "$SECRETS_FILE" ]; then
    echo -e "${RED}❌ 错误: Secrets 文件不存在: $SECRETS_FILE${NC}"
    echo ""
    echo -e "${YELLOW}请创建 secrets 文件:${NC}"
    cat << EOF
cat > $SECRETS_FILE << 'SECRETS'
DOCKERHUB_USERNAME=your_username
DOCKERHUB_TOKEN=your_token
SECRETS
EOF
    exit 1
fi
echo -e "${GREEN}✅ Secrets 文件存在${NC}"
echo ""

# 询问确认
if [ "$PUSH" = "true" ]; then
    echo -e "${YELLOW}⚠️  警告: 即将推送镜像到 Docker Hub!${NC}"
    read -p "确认继续? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo -e "${RED}❌ 已取消${NC}"
        exit 1
    fi
fi

# 运行 act
echo -e "${GREEN}🚀 开始构建...${NC}"
echo ""

# 记录开始时间
START_TIME=$(date +%s)

act workflow_dispatch \
    --secret-file "$SECRETS_FILE" \
    --input tag="$TAG" \
    --input push_to_registry="$PUSH" \
    -P ubuntu-latest=catthehacker/ubuntu:act-latest

# 记录结束时间
END_TIME=$(date +%s)

# 计算耗时
DURATION=$((END_TIME - START_TIME))
MINUTES=$((DURATION / 60))
SECONDS=$((DURATION % 60))

# 检查执行结果
if [ $? -eq 0 ]; then
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}✅ 构建成功!${NC}"
    echo -e "${GREEN}⏱️  总耗时: ${MINUTES} 分 ${SECONDS} 秒${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    if [ "$PUSH" = "true" ]; then
        echo -e "${GREEN}📦 镜像已推送到 Docker Hub${NC}"
    else
        echo -e "${YELLOW}💡 提示: 使用 -p 参数可推送到 Docker Hub${NC}"
    fi
else
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${RED}❌ 构建失败!${NC}"
    echo -e "${YELLOW}⏱️  总耗时: ${MINUTES} 分 ${SECONDS} 秒${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    exit 1
fi