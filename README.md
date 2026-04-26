# gapull

> /ˈɡæpʊl/ · *GitHub Actions Pull*

通过 GitHub Actions 在云端拉取 Docker 镜像，打包成 `.tar.gz`，下载到本地离线加载。
彻底解决国内网络无法直接 `docker pull` 的问题。

---

## 名称

| | |
|---|---|
| 全称 | **G**itHub **A**ctions **Pull** |
| 发音 | /ˈɡæpʊl/，"ga"如 *gap* 中的 /ɡæ/，"pull" 如 *pull* /pʊl/ |
| 快捷指令 | `dp`（**D**ocker **P**ull 缩写） |

---

## 原理

```
本地输入镜像名
    ↓
gapull 触发 GitHub Actions（云端网络）
    ↓
Runner: docker pull → docker save | gzip
    ↓
上传到 Release 或 Artifact
    ↓
gapull 多线程下载 → docker load
```

---

## 安装

```bash
curl -fsSL https://raw.githubusercontent.com/029527/gapull/master/install.sh | bash
```

脚本自动检测系统和架构（Linux/macOS × amd64/arm64），下载对应二进制到 `/usr/local/bin`，
并向当前 Shell 的配置文件注入快捷指令 `dp`。

**手动安装**：前往 [Releases](https://github.com/029527/gapull/releases/latest) 下载对应平台的二进制，
重命名为 `gapull` 并放入 `$PATH` 即可。

---

## 配置

安装完成后，配置一次 GitHub Token（需要 `workflow` 权限）：

```bash
gapull config set --token ghp_xxxxxxxxxxxxxxxx
```

获取 Token：GitHub → Settings → Developer settings → Personal access tokens → Tokens (classic) → 勾选 `workflow`

如果使用自己 fork 的仓库：

```bash
gapull config set --token ghp_xxx --owner your-name --repo gapull
```

---

## 使用

使 alias 生效后（或重启终端），直接用 `dp` 指令：

```bash
# 拉取单个镜像（自动检测当前系统架构，保存到当前目录）
dp nginx:latest

# 拉取多个镜像
dp nginx:latest,redis:7,alpine:3.20

# 指定架构
dp nginx:latest --arch arm64

# 下载到指定目录
dp nginx:latest --output ~/images

# 镜像大于 2GB 时使用 artifact 模式
dp pytorch/pytorch:2.3.0-cuda12.1-cudnn8-runtime --type artifact
```

不用 alias 的完整写法：

```bash
gapull pull nginx:latest
```

---

## 选择工作流类型

| 镜像大小 | 推荐类型 | 说明 |
|---|---|---|
| < 2 GB | `release`（默认） | 上传到 GitHub Release，下载无需认证 |
| 2 ~ 5 GB | `artifact` | 上传到 Actions Artifact，有效期 1 天 |
| > 5 GB | ❌ 不支持 | 请自行解决网络问题 |

支持架构：`amd64`（x86-64）、`arm64`（树莓派 4 等）、`arm32`（树莓派 3 等）。

---

## 加载镜像

```bash
docker load -i nginx_latest-amd64.tar.gz
```

> Artifact 模式下载的是 `.zip`，解压后得到 `.tar.gz`，再执行 `docker load`。

---

## 常见问题

**触发后 404？**
Token 需要 `workflow` 权限，且目标仓库必须是你自己的（fork 后使用）。

**架构不对？**
ARM 设备需指定 `--arch arm64`，不能加载 amd64 的镜像。

**下载慢？**
设置代理环境变量即可自动生效：`export HTTPS_PROXY=http://127.0.0.1:7890`
无代理时工具自动启用 16 线程 + 5MB 分块重试模式。

---

## License

[MIT](LICENSE)
