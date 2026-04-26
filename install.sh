#!/usr/bin/env bash
set -euo pipefail

REPO="029527/gapull"
BIN="gapull"
INSTALL_DIR="/usr/local/bin"
ALIAS_CMD="alias dp='gapull pull'"

# ── 颜色输出 ──────────────────────────────────────────────
green()  { printf '\033[32m%s\033[0m\n' "$*"; }
yellow() { printf '\033[33m%s\033[0m\n' "$*"; }
red()    { printf '\033[31m%s\033[0m\n' "$*"; }
die()    { red "Error: $*"; exit 1; }

# ── 检测 OS ───────────────────────────────────────────────
case "$(uname -s)" in
  Linux)  OS="linux"  ;;
  Darwin) OS="darwin" ;;
  *)      die "不支持的操作系统: $(uname -s)" ;;
esac

# ── 检测架构 ──────────────────────────────────────────────
case "$(uname -m)" in
  x86_64)          ARCH="amd64" ;;
  aarch64 | arm64) ARCH="arm64" ;;
  *)               die "不支持的架构: $(uname -m)" ;;
esac

ASSET="${BIN}-${OS}-${ARCH}"

# ── 获取最新版本号 ─────────────────────────────────────────
LATEST_TAG=""
if command -v curl &>/dev/null; then
  LATEST_TAG=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
    "https://github.com/${REPO}/releases/latest" 2>/dev/null \
    | sed 's|.*/tag/||')
elif command -v wget &>/dev/null; then
  LATEST_TAG=$(wget -q --server-response --spider \
    "https://github.com/${REPO}/releases/latest" 2>&1 \
    | grep -i "Location:" | tail -1 | sed 's|.*/tag/||' | tr -d '[:space:]')
fi

URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "检测到: ${OS}/${ARCH}"
if [[ -n "${LATEST_TAG}" ]]; then
  green "最新版本: ${LATEST_TAG}"
fi
echo ""

# ── 已安装检测 ────────────────────────────────────────────
INSTALLED_BIN="$(command -v "${BIN}" 2>/dev/null || true)"
if [[ -n "$INSTALLED_BIN" ]]; then
  CURRENT_VER="$("${INSTALLED_BIN}" --version 2>/dev/null | awk '{print $NF}' || echo "未知版本")"
  yellow "检测到已安装的 ${BIN}（${CURRENT_VER}，位于 ${INSTALLED_BIN}）"
  echo ""
  echo "请选择操作："
  echo "  1) 升级到最新版本${LATEST_TAG:+（${LATEST_TAG}）}"
  echo "  2) 卸载"
  echo "  3) 取消"
  echo ""
  read -r -p "输入选项 [1/2/3]: " CHOICE </dev/tty
  case "$CHOICE" in
    1) echo "" && green "开始升级..." ;;
    2)
      echo ""
      if [[ -w "$(dirname "$INSTALLED_BIN")" ]]; then
        rm -f "$INSTALLED_BIN"
      else
        sudo rm -f "$INSTALLED_BIN"
      fi
      # 清理 alias
      for rc in "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.config/fish/config.fish"; do
        [[ -f "$rc" ]] || continue
        if grep -qF "gapull" "$rc" 2>/dev/null; then
          grep -vE "^# gapull shortcut$|^alias dp.+gapull" "$rc" > "${rc}.tmp" && mv "${rc}.tmp" "$rc"
          yellow "已清理 alias → ${rc}"
        fi
      done
      green "${BIN} 已卸载。"
      exit 0
      ;;
    *)
      yellow "已取消。"
      exit 0
      ;;
  esac
fi

echo "下载: ${URL}"
echo ""

# ── 下载 ──────────────────────────────────────────────────
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

if command -v curl &>/dev/null; then
  curl -fsSL "$URL" -o "$TMP"
elif command -v wget &>/dev/null; then
  wget -qO "$TMP" "$URL"
else
  die "需要 curl 或 wget，请先安装其中一个"
fi

chmod +x "$TMP"

# ── 安装到 /usr/local/bin ─────────────────────────────────
if [[ -w "$INSTALL_DIR" ]]; then
  mv "$TMP" "${INSTALL_DIR}/${BIN}"
else
  yellow "需要 sudo 权限写入 ${INSTALL_DIR}"
  sudo mv "$TMP" "${INSTALL_DIR}/${BIN}"
fi

green "已安装: ${INSTALL_DIR}/${BIN}"

# ── 注入 alias dp ─────────────────────────────────────────
inject_alias() {
  local rc="$1"
  local alias_str="$2"

  [[ -f "$rc" ]] || return 0

  if grep -qF "$alias_str" "$rc" 2>/dev/null; then
    yellow "alias 已存在于 ${rc}，跳过"
    return 0
  fi

  printf '\n# gapull shortcut\n%s\n' "$alias_str" >> "$rc"
  green "已写入 alias → ${rc}"
}

SHELL_NAME="$(basename "${SHELL:-}")"

case "$SHELL_NAME" in
  zsh)
    inject_alias "$HOME/.zshrc" "$ALIAS_CMD"
    RC="$HOME/.zshrc"
    ;;
  bash)
    if [[ "$OS" == "darwin" ]]; then
      inject_alias "$HOME/.bash_profile" "$ALIAS_CMD"
      RC="$HOME/.bash_profile"
    else
      inject_alias "$HOME/.bashrc" "$ALIAS_CMD"
      RC="$HOME/.bashrc"
    fi
    ;;
  fish)
    FISH_RC="$HOME/.config/fish/config.fish"
    inject_alias "$FISH_RC" "alias dp 'gapull pull'"
    RC="$FISH_RC"
    ;;
  *)
    yellow "未识别的 Shell（${SHELL_NAME}），请手动添加："
    yellow "  ${ALIAS_CMD}"
    RC=""
    ;;
esac

# ── 完成提示 ──────────────────────────────────────────────
echo ""
green "安装完成！"
echo ""
echo "使用前先配置 GitHub Token（只需一次）："
echo "  gapull config set --token <your-PAT>"
echo ""
if [[ -n "${RC:-}" ]]; then
  echo "使新 alias 生效："
  echo "  source ${RC}"
  echo ""
fi
echo "之后即可使用快捷指令："
echo "  dp nginx:latest"
echo "  dp nginx:latest,redis:7"
