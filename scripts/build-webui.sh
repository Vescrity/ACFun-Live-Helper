#!/usr/bin/env bash
# ===========================================================================
# build-webui.sh — ACFun Live Helper WebUI 打包脚本
#
# 作用：一步完成前端构建 + Go 编译 + 打包，产出独立可用的单文件二进制。
#      也支持交叉编译 Windows 目标。
#
# 用法：
#   ./scripts/build-webui.sh                  # 为本机架构编译
#   ./scripts/build-webui.sh --target win     # 交叉编译 Windows amd64
#   ./scripts/build-webui.sh --output ./dist  # 指定输出目录
#   ./scripts/build-webui.sh --help           # 查看全部选项
# ===========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$SCRIPT_DIR"

# ---- 配置 ----
BINARY_NAME="aclivehelper-webui"
OUTPUT_DIR="${SCRIPT_DIR}/build"
BUILD_DIR="${OUTPUT_DIR}/bin"
VERSION="$(node -p "require('./package.json').version" 2>/dev/null || echo "unknown")"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
BUILD_TIME="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"

# ---- 解析参数 ----
TARGET_OS=""
TARGET_ARCH=""
PARALLEL_JOBS=1

while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h)
            echo "用法: $0 [选项]"
            echo ""
            echo "选项:"
            echo "  --target <os/arch>   目标平台 (default: current)"
            echo "                        示例: linux/amd64, windows/amd64, linux/arm64"
            echo "  --output <dir>       输出目录前缀 (default: ./build)"
            echo "  --parallel <n>       npm install 并行数 (default: 1)"
            echo "  -j <n>               同上"
            echo "  --skip-npm           跳过 npm install（构建已存在时加速）"
            echo "  --skip-build         跳过 Go 编译（仅构建前端）"
            echo "  --help, -h           显示此帮助"
            exit 0
            ;;
        --target)
            shift
            IFS='/' read -r TARGET_OS TARGET_ARCH <<< "$1"
            if [[ -z "$TARGET_ARCH" ]]; then
                TARGET_ARCH="$TARGET_OS"
                TARGET_OS=""
            fi
            ;;
        --output)
            shift
            OUTPUT_DIR="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")" 2>/dev/null || OUTPUT_DIR="$1"
            BUILD_DIR="${OUTPUT_DIR}/bin"
            ;;
        --parallel|-j)
            shift
            PARALLEL_JOBS="$1"
            ;;
        --skip-npm)
            SKIP_NPM=1
            ;;
        --skip-build)
            SKIP_BUILD=1
            ;;
        *)
            echo "未知选项: $1"
            echo "使用 --help 查看帮助"
            exit 1
            ;;
    esac
    shift
done

# ---- 环境检测 ----
echo "🔍 环境检测..."

command -v node >/dev/null 2>&1 || { echo "❌ 需要 Node.js 18+"; exit 1; }
command -v npm  >/dev/null 2>&1 || { echo "❌ 需要 npm"; exit 1; }
command -v go   >/dev/null 2>&1 || { echo "❌ 需要 Go 1.21+"; exit 1; }

NODE_VER="$(node -v | sed 's/v//;s/\..*//')"
GO_VER="$(go version | sed 's/.*go\([0-9.]*\).*/\1/')"
echo "   Node.js: $(node -v)"
echo "   npm:     $(npm -v)"
echo "   Go:      $GO_VER"
echo "   版本:    $VERSION ($COMMIT)"

# ---- 构建前端 ----
if [[ -z "${SKIP_NPM:-}" ]]; then
    echo ""
    echo "📦 安装前端依赖..."
    npm install 2>&1 | tail -1
fi

echo ""
echo "🏗️  构建前端 (Vite)..."
npm run build 2>&1
echo "   ✅ dist/ 构建完成"

# ---- 编译 Go 二进制 ----
if [[ -z "${SKIP_BUILD:-}" ]]; then
    echo ""
    echo "🦦 编译 Go 二进制..."

    mkdir -p "$BUILD_DIR"

    GO_FLAGS=(-trimpath -ldflags="-s -w")

    # 版本信息注入
    if [[ -n "$VERSION" ]]; then
        GO_FLAGS=(-trimpath -ldflags="-s -w -X main.version=$VERSION")
    fi

    if [[ -n "$TARGET_OS" ]]; then
        export GOOS="$TARGET_OS"
        echo "   目标 OS:   $GOOS"
    fi
    if [[ -n "$TARGET_ARCH" ]]; then
        export GOARCH="$TARGET_ARCH"
        echo "   目标 ARCH: $GOARCH"
    fi
    echo "   当前 OS/ARCH: $(go env GOOS)/$(go env GOARCH)"

    # 交叉编译 Windows 时禁用 CGO（WebUI 模式不需要 CGO）
    export CGO_ENABLED=0

    OUTPUT_NAME="$BINARY_NAME"
    if [[ "$(go env GOOS)" = "windows" ]]; then
        OUTPUT_NAME="${BINARY_NAME}.exe"
    fi

    echo "   编译中..."
    go build "${GO_FLAGS[@]}" -o "${BUILD_DIR}/${OUTPUT_NAME}" .

    echo "   ✅ 编译完成: ${BUILD_DIR}/${OUTPUT_NAME}"
    ls -lh "${BUILD_DIR}/${OUTPUT_NAME}"

    # ---- 打包 ----
    echo ""
    echo "📦 打包..."

    PLATFORM="$(go env GOOS)-$(go env GOARCH)"
    PACKAGE_NAME="aclivehelper-${VERSION}-webui-${PLATFORM}"

    # 创建临时打包目录
    PKG_DIR="$(mktemp -d)"
    cp "${BUILD_DIR}/${OUTPUT_NAME}" "${PKG_DIR}/"

    # 生成校验文件
    (cd "$PKG_DIR" && sha256sum "$OUTPUT_NAME" > "${OUTPUT_NAME}.sha256")

    if [[ "$(go env GOOS)" = "windows" ]]; then
        # Windows: zip 格式
        command -v zip >/dev/null 2>&1 && {
            (cd "$PKG_DIR" && zip -q "${SCRIPT_DIR}/${PACKAGE_NAME}.zip" ./*)
            echo "   ✅ ${PACKAGE_NAME}.zip"
        } || echo "   ⚠️  zip 未安装，跳过打包"
    else
        # Linux/macOS: tar.gz 格式
        tar -czf "${SCRIPT_DIR}/${PACKAGE_NAME}.tar.gz" -C "$PKG_DIR" .
        echo "   ✅ ${PACKAGE_NAME}.tar.gz"
        ls -lh "${SCRIPT_DIR}/${PACKAGE_NAME}.tar.gz"
    fi

    rm -rf "$PKG_DIR"

    echo ""
    echo "🎉 全部完成！"
    echo "   二进制: ${BUILD_DIR}/${OUTPUT_NAME}（单文件，复制即用）"
    if [[ -f "${SCRIPT_DIR}/${PACKAGE_NAME}.tar.gz" ]]; then
        echo "   压缩包: ${SCRIPT_DIR}/${PACKAGE_NAME}.tar.gz"
    fi
else
    echo ""
    echo "✅ 前端构建完成（--skip-build），dist/ 已就绪"
fi
