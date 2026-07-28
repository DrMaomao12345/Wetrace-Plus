#!/usr/bin/env bash
# Wetrace-Pro · macOS 一键启动脚本
#
# 用法:  ./run-macos.sh
# 环境:  需要 Go(brew install go)+ Xcode 命令行工具(提供 lldb)
#
# 提取微信数据库密钥需要:①微信已登录运行 ②SIP 已关闭(恢复模式 csrutil disable)
# 在网页「系统配置与密钥 → 获取密钥」时,请在微信里切换几个聊天/打开朋友圈以触发。

set -euo pipefail
export PATH="/opt/homebrew/bin:$PATH"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

ROOT="$(cd "$(dirname "$0")" && pwd)"
RUN_DIR="${WETRACE_RUN_DIR:-$HOME/.wetrace}"
PORT="${WETRACE_PORT:-5333}"
BIN="$RUN_DIR/wetrace-mac"
mkdir -p "$RUN_DIR"

echo "== 环境自检 =="
if ! command -v go >/dev/null 2>&1; then
  echo "❌ 未找到 go,请先 brew install go"; exit 1
fi
if ! csrutil status 2>/dev/null | grep -qi disabled; then
  echo "⚠️  SIP 未关闭 —— 之后「获取密钥」会失败。需在恢复模式执行 csrutil disable。"
fi
pgrep -x WeChat >/dev/null 2>&1 || echo "⚠️  未检测到微信在运行,请先登录微信再获取密钥。"

# 增量编译:源码比二进制新时才重编
if [ ! -x "$BIN" ] || [ "$ROOT/main.go" -nt "$BIN" ]; then
  echo "== 编译 macOS 二进制 =="
  ( cd "$ROOT" && CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o "$BIN" . )
fi

cd "$RUN_DIR"
[ -f .env ] || printf 'LISTEN_ADDR=127.0.0.1:%s\n' "$PORT" > .env

# 若已在运行,先停掉旧实例
pkill -x wetrace-mac 2>/dev/null || true

echo "== 启动 Wetrace =="
echo "   访问: http://127.0.0.1:$PORT"
echo "   数据/配置目录: $RUN_DIR"
exec "$BIN"
