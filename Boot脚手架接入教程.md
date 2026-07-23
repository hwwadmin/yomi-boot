# Boot 脚手架接入教程

业务工程依赖公司内部基础设施库 `yt-boot` 的发版与接入说明。

文中 Git 地址以当前内网环境为准。若入口变更，按实际地址替换即可。

| 项 | 值 |
|----|-----|
| module 名 | `yt-boot` |
| 业务代码 import | `"yt-boot/..."`（例如 `"yt-boot/bootstrap"`） |
| Git 入口（Go 模块） | `https://git.internal` |
| 仓库路径 | `xy_ai_hoyi/yt-boot` |
| clone 地址 | `https://git.internal/xy_ai_hoyi/yt-boot.git` |
| remote `replace` | `git.internal/xy_ai_hoyi/yt-boot` |

内网需将 `git.internal` 解析到 Git 机（当前为 `192.168.110.203`），例如本机 hosts：

```text
192.168.110.203  git.internal
```

公司 GitLab 网页 / 原有入口仍为 `http://link.maiip.net:3000`（不依赖本教程的 hosts）。

---

## 1. boot 发版

在 `yt-boot` 仓库打 SemVer tag 并推送：

```bash
git tag v1.0.0
git push origin v1.0.0
```

业务仓升级时同步修改 `require` / `replace` 中的版本号。

---

## 2. 业务仓接入

提供两种方式：

| 方式 | 适用 | 说明 |
|------|------|------|
| **方式一** | 正式依赖已发布的 tag | `require` + remote `replace`，由 Go 经 go-get 后 git 拉取 |
| **方式二** | 本地联调未发版代码 | 先 clone 到本地目录，`replace` 指向该目录 |

### 方式一：`require` + remote `replace`

#### 环境准备（开发机 / CI）

**hosts：** 确保 `git.internal` 指向 Git 机 IP。

**Go：**

```bash
go env -w GOPRIVATE=git.internal
go env -w GONOSUMDB=git.internal
go env -w GONOPROXY=git.internal
go env -w GOINSECURE=git.internal
```

**Git：**

入口为 HTTPS 自签证书，私有仓需配置只读凭据。

```bash
git config --global http.https://git.internal/.sslVerify false
```

在用户 home 下配置 netrc（Linux/macOS：`~/.netrc`；Windows：`%USERPROFILE%\_netrc`）：

```text
machine git.internal
login oauth2
password <gitlab-personal-access-token>
```

Token 需具备 `read_repository` 权限。也可用 GitLab 用户名作为 `login`，`password` 填 Token。

#### 环境校验

**1）hosts**

```bash
ping -c 1 git.internal
# Windows: ping -n 1 git.internal
# 期望解析到 Git 机 IP（当前为 192.168.110.203）
```

**2）Go 环境**

```bash
go env GOPRIVATE GONOSUMDB GONOPROXY GOINSECURE
```

期望四项均包含 `git.internal`（可与其它前缀用逗号并存，但必须包含该项）。

**3）Git sslVerify**

```bash
git config --global --get http.https://git.internal/.sslVerify
```

期望输出：`false`。

**4）凭据 + HTTPS 可达**

```bash
git ls-remote https://git.internal/xy_ai_hoyi/yt-boot.git HEAD
```

期望打印一行 commit（如 `76d6d545...  HEAD`），无 `Authentication failed` / `x509` / `Could not resolve host`。

若失败：检查 netrc 主机名是否为 `git.internal`、Token 是否有效、hosts 是否生效。

#### 业务仓 `go.mod`

```go
require yt-boot v1.0.0

replace yt-boot => git.internal/xy_ai_hoyi/yt-boot v1.0.0
```

```bash
go mod tidy
```

> module path **不能**包含端口。

#### 自检

```bash
curl -sk "https://git.internal/xy_ai_hoyi/yt-boot?go-get=1"
# 期望包含：
# go-import ... git.internal/xy_ai_hoyi/yt-boot git https://git.internal/xy_ai_hoyi/yt-boot.git

git ls-remote https://git.internal/xy_ai_hoyi/yt-boot.git
```

---

### 方式二：clone 后 `replace` 本地目录

```bash
git clone --branch v1.0.0 --depth 1 \
  https://git.internal/xy_ai_hoyi/yt-boot.git ../yt-boot
```

```go
require yt-boot v1.0.0

replace yt-boot => ../yt-boot
```

```bash
go mod tidy
```

路径按业务仓与 `yt-boot` 的实际相对位置调整；CI 可先 clone 到约定目录再使用本地 `replace`。

---

## 3. 基础设施约定

| 项 | 说明 |
|----|------|
| 公司 GitLab（勿改） | `http://link.maiip.net:3000`，内外网共用 |
| 内网 Go 入口 | `https://git.internal`（203 上 Nginx 80/443 → GitLab `:3000`） |
| go-get | 由 Nginx **写死返回** `go-import`（前缀 `git.internal/...`，仓库 URL 为 HTTPS） |
| DNS / hosts | `git.internal` → Git 机 IP（当前 `192.168.110.203`） |

业务仓通过 `git.internal` 拉模块时只依赖 203 的 Nginx；不要求修改 GitLab Omnibus 配置。

---

## 4. 排障

| 现象 | 原因 | 处理 |
|------|------|------|
| 无法解析 `git.internal` | 未配置 hosts / DNS | 增加 `192.168.110.203 git.internal` |
| `Unencrypted HTTP is not recommended for GitLab` | 经 HTTP 拉取且启用了 Git Credential Manager | 使用 HTTPS 仓库地址 |
| `could not read Username ... terminal prompts disabled` | 私有仓无凭据，且非交互环境 | 配置 netrc 或 Credential Manager |
| `x509` / SSL 校验失败 | 自签证书不受信 | `http.https://git.internal/.sslVerify false`，并保留 `GOINSECURE` |
| `malformed import path ... invalid char ':'` | module path 中写了端口 | 使用 `git.internal/xy_ai_hoyi/yt-boot`（无端口） |
| `?go-get=1` 失败或前缀不匹配 | Nginx meta 与 `replace` 主机名不一致 | 确认 meta 前缀为 `git.internal/...` |
