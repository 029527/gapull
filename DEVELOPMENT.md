# 开发指南

## 环境要求

- Go 1.21+
- [GitHub CLI (`gh`)](https://cli.github.com/) — 发布 Release 时需要已登录：`gh auth login`

## 常用命令

### 构建

编译全平台二进制到 `dist/`：

```bash
make build
# 或直接 make
```

产物：

```
dist/
  gapull-linux-amd64
  gapull-linux-arm64
  gapull-darwin-amd64
  gapull-darwin-arm64
```

清理产物：

```bash
make clean
```

### 发布新版本

一条命令完成构建 + 打 tag + 创建 GitHub Release + 上传四个二进制：

```bash
make release TAG=v1.0.0
```

> `gh release create` 会自动生成 Release Notes（基于提交记录），并将 `dist/*` 全部上传。

### 只构建不发布

```bash
make build
```

之后可以手动上传，或用 `gh` 补传到已有 Release：

```bash
gh release upload v1.0.0 dist/* --repo 029527/gapull
```

## 发版流程（标准步骤）

```
1. 开发 & 提交
   git add .
   git commit -m "feat: xxx"
   git push

2. 确认无误后发版
   make release TAG=v1.2.0

3. 验证
   gh release view v1.2.0 --repo 029527/gapull
```

## 重新打 tag（已有 Release 需要修补）

```bash
# 删除本地 tag
git tag -d v1.0.0

# 删除远端 tag
git push origin :refs/tags/v1.0.0

# 删除旧 Release（可选，否则 gh release create 会报冲突）
gh release delete v1.0.0 --repo 029527/gapull --yes

# 重新发版
make release TAG=v1.0.0
```

## 安装脚本测试

本地验证 install.sh 逻辑（dry-run，不实际写入 /usr/local/bin）：

```bash
bash -x install.sh
```

## 目录结构

```
.
├── cmd/            # cobra 子命令（config、pull 等）
├── internal/       # 内部包
├── main.go         # 入口
├── Makefile        # build / clean / release
├── install.sh      # 一键安装脚本
└── dist/           # 编译产物（git 忽略）
```
