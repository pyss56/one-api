# One API 安全审计报告

- **审计对象**：One API（songquanpeng/one-api）
- **审计版本**：`main` 分支，commit `8df4a26`
- **审计日期**：2026-08-27
- **审计方式**：源码逐行审查（代码已 clone 至本地 `d:/project/llm_proxy/one-api`）
- **审计范围**：认证/授权、用户与权限、token 与兑换码、转发层（relay）、第三方 OAuth 登录、CORS、网络/IP 提取、请求体处理、密钥管理、配置初始化

---

## 一、总体结论

One API 整体代码质量可控，**授权模型（RBAC + token 网段限制）设计合理**，且**自建 token 不会泄漏给上游 provider**（已通过 `distributor.go:73` 覆盖 Authorization 验证）。

但存在 **1 个官方确认的高危业务漏洞（无修复）** + **若干部署相关风险**。对内网代理场景，多数可控，但务必遵守本报告「缓解措施」。

| 风险等级 | 数量 | 说明 |
|---------|------|------|
| 高危（业务漏洞） | 1 | 兑换码并发重复兑换（CVE-2026-11465），仅 MySQL |
| 中危 | 3 | XSS、请求体/解压 DoS、CORS 全开 |
| 低危/配置项 | 3 | 默认密码、调试模式关限流、日志含请求体 |
| 已确认安全 | 4 | 密钥不泄漏、IP 不可伪造绕过网段、密码 bcrypt、OAuth state 校验 |

---

## 二、已确认漏洞（逐项附源码证据）

### [高危] 1. 兑换码并发重复兑换 — CVE-2026-11465 / GO-2026-6126（Issue #2397，无官方修复）

**文件**：`model/redemption.go`

```go
// redemption.go:69
err := tx.Set("gorm:query_option", "FOR UPDATE").
    Where(keyCol+" = ?", key).First(redemption).Error
```

**问题**：GORM v2 不识别 `gorm:query_option` 这个 key，导致 `FOR UPDATE` 行锁被**静默丢弃**。并发请求可同时读取到同一未兑换的兑换码并各自兑换成功。

- **影响范围**：仅 **MySQL** 后端（SQLite 使用不同原子路径，不受影响）。
- **危害**：普通账号（`RoleCommonUser`）即可并发刷取额度，直接消耗你的真实 LLM 余额，无需管理员权限、无需改代码。
- **触发门槛**：极低。
- **官方状态**：无修复版本（pkg.go.dev 标注 affected）。

**缓解**：
1. 使用 **SQLite** 而非 MySQL；或
2. 不启用兑换码 / 充值（topup）功能；或
3. 若必须用 MySQL，需自行打补丁（见附录）。

---

### [中危] 2. 存储型 XSS — CVE-2025-3801 / GO-2025-3636

**文件**：`web/default/src/pages/Home/index.js:290`、`web/berry/src/pages/Home/index.js:295`、`web/air/src/pages/Home/index.js:388`

```js
<div dangerouslySetInnerHTML={{ __html: homePageContent }}></div>
```

`homePageContent` 来自后端 `controller/misc.go` 的 `HomePageContent` 字段，经 `marked.parse` 后原样注入 DOM，后端 `controller/misc.go:74` 未做任何清洗（无 DOMPurify / 转义）。

- **触发条件**：需管理员权限修改「系统设置 → 首页内容」。
- **危害**：内网可信管理员场景风险低；若管理后台暴露给不可信用户则可导致会话劫持。

**缓解**：
1. 首页内容仅填写纯文本，勿含 HTML/JS；或
2. 反向代理（Nginx）对该路由加 `Content-Security-Policy` 头。

---

### [中危] 3. 请求体无限大小 + gzip 解压炸弹（DoS）

**文件**：`common/gin.go:18`、`middleware/gzip.go:13`

```go
// common/gin.go
requestBody, err := io.ReadAll(c.Request.Body)   // 无 MaxBytesReader 限制
// middleware/gzip.go
gzipReader, err := gzip.NewReader(c.Request.Body) // 解压后无大小限制
```

- 整个请求体被完整读入内存，无 `MaxBytesReader` / `http.MaxBytesReader` 限制（`FileUploadMaxBytes` 常量仅用于头像上传 `user.go`，relay 路径未使用）。
- 配合 `Content-Encoding: gzip` 可构造解压炸弹，打满内存。

**缓解**：在反向代理层（Nginx）限制 `client_max_body_size` 与请求速率；内网场景影响有限。

---

### [中危] 4. CORS 全开且允许凭证

**文件**：`middleware/cors.go:10-11`

```go
config.AllowAllOrigins = true
config.AllowCredentials = true
```

- API 网关场景影响较小（调用方本就需携带 key）。
- 但若**管理后台与 API 同源部署**，会放大 XSS / CSRF 风险。

**缓解**：生产环境用 Nginx 覆盖 CORS 策略，仅允许内网源；管理后台与 API 分离部署。

---

### [低危] 5. 默认管理员密码 `123456`

**文件**：初始化逻辑（`model/main.go` / 首次启动创建 root 用户）

- 首次启动若未配置，root 默认密码为 `123456`。
- **危害**：若管理后台暴露，任何人可接管网关、读取全部 provider 真实 key。

**缓解**：部署后立即修改 root 密码；通过环境变量 `SESSION_SECRET` 固定会话密钥。

---

### [低危] 6. Debug 模式下游率限制失效

**文件**：`middleware/rate-limit.go`

```go
if config.DebugEnabled {
    c.Next()
    return
}
```

- 调试模式下所有限流被绕过。

**缓解**：生产部署务必关闭 `DebugEnabled`（默认 false，勿手动开启）。

---

### [低危] 7. 异常时请求体写入日志

**文件**：`middleware/recover.go`

- panic 恢复中间件将 `request body` 打印到日志。
- token 在 `Authorization` 头（不在 body），影响小，但建议日志脱敏。

---

## 三、重点安全点验证（确认为安全）

### ✅ 自建 token 不会泄漏给上游 provider
**证据链**：
- `middleware/distributor.go:73`：`c.Request.Header.Set("Authorization", "Bearer "+channel.Key)` —— 在分发阶段用 **channel 真实 key** 覆盖了请求头的 Authorization。
- `relay/meta/relay_meta.go:52`：`meta.APIKey = strings.TrimPrefix(c.Request.Header.Get("Authorization"), "Bearer ")` —— 此时读到的已是被覆盖后的 channel.Key。
- `relay/adaptor/openai/adaptor.go:76`：`req.Header.Set("Authorization", "Bearer "+meta.APIKey)` —— 上游收到的是 channel.Key。
- `relay/controller/audio.go:151/156`：读取 `c.Request.Header.Get("Authorization")` 发生在覆盖之后，同样得到 channel.Key。

→ **结论：客户端自建 token 不会出现在任何对上游的请求中。**

### ✅ IP 不可伪造绕过网段限制
- 全局搜索 `SetTrustedProxies` 结果为 **0**，Gin 默认 `TrustedProxies = nil`，`c.ClientIP()` 直接取 `RemoteAddr`，不信任 `X-Forwarded-For`。
- `token.Subnet` 网段限制（`middleware/auth.go:104`）无法被 `X-Forwarded-For` 伪造绕过。
- **前提**：部署时不要调用 `c.SetTrustedProxies(...)` 把代理设为可信。

### ✅ 密码使用 bcrypt
`common/crypto.go`（`Password2Hash` 使用 bcrypt），非明文 / 弱哈希（md5/sha1）。

### ✅ OAuth state 校验防 CSRF
`controller/auth/github.go`、`oidc.go` 均校验 `state == session.Get("oauth_state")`，oauth_state 为随机字符串。

### ✅ 敏感字段不回显
`model/user.go` 中 `GetUserById(selectAll=false)` 使用 `Omit("password","access_token")`；列表/搜索接口同样 Omit password。

### ✅ 授权模型（RBAC）合理
guest / common / admin / root 四级；越权检查 `myRole <= originUser.Role` 逻辑正确，未发现越权漏洞。

---

## 四、对你「内网 AI 代理」场景的部署建议

1. **使用 SQLite，不要使用 MySQL** → 规避最高危的并发兑换漏洞（CVE-2026-11465）。
2. **部署后立即修改 root 默认密码 `123456`**。
3. **管理后台不对外网暴露**，仅暴露 `/v1` API 端口给业务项目。
4. **为 token 设置 `Subnet` 字段绑定内网网段** → 实现「泄露也出不了内网」（已验证无法被伪造 IP 绕过）。
5. **前置 Nginx**：限制请求体大小、加速率限制、收紧 CORS、给管理后台加 CSP。
6. **关闭 Debug 模式**，固定 `SESSION_SECRET` 环境变量。
7. **首页内容不填 HTML/JS**，规避 XSS。
8. **不启用兑换码 / 充值功能**（除非已打补丁）。

---

## 五、附录：MySQL 场景下的修复补丁（可选）

若必须使用 MySQL，将 `model/redemption.go:69` 改为 GORM v2 正确的行锁写法：

```go
import "gorm.io/gorm/clause"

err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
    Where(keyCol+" = ?", key).First(redemption).Error
```

并在兑换成功后显式 `tx.Commit()`，失败 `tx.Rollback()`，确保事务原子性。

---

*本报告基于本地 clone 的源码逐行审计，覆盖认证、授权、转发、OAuth、CORS、密钥管理、配置等核心模块。relay/adaptor 下 152 个 provider 适配文件中，已抽样审计 openai / tencent / ali / replicate 等代表性实现，未发现除上文所列之外的密钥泄漏或 SSRF 问题（所有上游 URL 来自管理员配置的 `Channel.BaseURL`，普通 token 持有者无法注入任意主机）。*
