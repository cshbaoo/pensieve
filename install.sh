#!/usr/bin/env sh
# pensieve 一键安装:curl -fsSL https://raw.githubusercontent.com/cshbaoo/pensieve/main/install.sh | sh
# 环境变量覆盖:VERSION=v0.3.0 / PENSIEVE_INSTALL=/usr/local/bin
set -eu

REPO="cshbaoo/pensieve"
INSTALL_DIR="${PENSIEVE_INSTALL:-}"

say() { printf '%s\n' "$*"; }
die() { say "✘ $*" >&2; exit 1; }

# ---- 平台检测 ----
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$OS" in
  linux|darwin) ;;
  mingw*|msys*|cygwin*) die "Windows 请用: irm https://raw.githubusercontent.com/$REPO/main/install.ps1 | iex" ;;
  *) die "不支持的 OS: $OS" ;;
esac
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) die "不支持的架构: $ARCH" ;;
esac

# ---- 版本解析:缺省取 GitHub 最新 release ----
VERSION="${VERSION:-}"
if [ -z "$VERSION" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')" \
    || die "无法获取最新 release(也可 VERSION=vX.Y.Z 显式指定)"
fi
[ -n "$VERSION" ] || die "无法解析最新版本号"
say "安装 pensieve $VERSION ($OS/$ARCH) ..."

# ---- 下载 + 校验 ----
ASSET="pensieve-$OS-$ARCH"
URL="https://github.com/$REPO/releases/download/$VERSION/$ASSET"
SUMS_URL="https://github.com/$REPO/releases/download/$VERSION/checksums.txt"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
curl -fsSL "$URL" -o "$TMP/$ASSET" || die "下载失败: $URL"
if command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1; then
  curl -fsSL "$SUMS_URL" -o "$TMP/checksums.txt" || die "下载校验清单失败"
  EXPECTED="$(grep " $ASSET\$" "$TMP/checksums.txt" | awk '{print $1}')"
  [ -n "$EXPECTED" ] || die "校验清单里没有 $ASSET"
  if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL="$(sha256sum "$TMP/$ASSET" | awk '{print $1}')"
  else
    ACTUAL="$(shasum -a 256 "$TMP/$ASSET" | awk '{print $1}')"
  fi
  [ "$EXPECTED" = "$ACTUAL" ] || die "SHA256 校验失败!expected=$EXPECTED actual=$ACTUAL"
  say "✔ 校验通过"
else
  say "⚠ 系统无 sha256sum/shasum,跳过校验(建议安装 coreutils 后重试)"
fi

# ---- 安装 ----
if [ -z "$INSTALL_DIR" ]; then
  if [ -w /usr/local/bin ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi
mkdir -p "$INSTALL_DIR"
cp "$TMP/$ASSET" "$INSTALL_DIR/pensieve"
chmod +x "$INSTALL_DIR/pensieve"

say "✔ 已安装: $INSTALL_DIR/pensieve"
"$INSTALL_DIR/pensieve" --help >/dev/null 2>&1 && say "✔ 可执行正常" || say "⚠ 二进制未能自举,请检查 $OS/$ARCH 匹配"
if ! command -v pensieve >/dev/null 2>&1; then
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) say "提示: $INSTALL_DIR 不在 PATH。加入: export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
  esac
fi
say "下一步: pensieve init(初始化记忆仓库) → 在 AGENTS.md 所在仓库接入 MCP: pensieve serve"
