# Sub2API Lite 模式转换进度总结

**生成时间**: 2026-01-11
**最后更新**: 2026-04-08
**状态**: 上游同步 v0.1.109 完成
**Light 分支**: https://github.com/Kevin-Lab777/sub2api/tree/Light

## Phase 1: 删除/简化代码文件 ✅ 完成

### Handler 层

**完全删除**:
- [x] `internal/handler/admin/redeem_handler.go` - 删除
- [x] `internal/handler/admin/promo_handler.go` - 删除
- [x] `internal/handler/admin/user_attribute_handler.go` - 删除
- [x] `internal/handler/redeem_handler.go` - 删除
- [x] `internal/handler/subscription_handler.go` - 删除 (用户端)
- [x] `internal/handler/auth_linuxdo_oauth.go` - 删除

**简化保留** (有 [LITE] 标记):
- [x] `internal/handler/auth_handler.go` - 简化保留 (只保留 Login + GetCurrentUser)
- [x] `internal/handler/user_handler.go` - 简化保留 (Profile 相关)
- [x] `internal/handler/api_key_handler.go` - 简化保留 (完整 CRUD)
- [x] `internal/handler/usage_handler.go` - 简化保留 (完整功能)
- [x] `internal/handler/admin/user_handler.go` - 简化保留 (无用户属性)

### Service 层

**完全删除**:
- [x] `internal/service/identity_service.go`
- [x] `internal/service/turnstile_service.go`
- [x] `internal/service/email_service.go`
- [x] `internal/service/email_queue_service.go`
- [x] `internal/service/redeem_service.go`
- [x] `internal/service/redeem_code.go`
- [x] `internal/service/promo_service.go`
- [x] `internal/service/promo_code.go`
- [x] `internal/service/promo_code_repository.go`
- [x] `internal/service/user_attribute.go`
- [x] `internal/service/user_attribute_service.go`
- [x] `internal/service/ops_alert_evaluator_service.go` - 依赖 EmailService
- [x] `internal/service/ops_scheduled_report_service.go` - 依赖 EmailService

**简化保留** (有 [LITE] 标记):
- [x] `internal/service/auth_service.go` - 简化保留 (只保留 Login + ValidatePassword)

### Repository 层
- [x] `internal/repository/email_cache.go`
- [x] `internal/repository/identity_cache.go`
- [x] `internal/repository/promo_code_repo.go`
- [x] `internal/repository/redeem_cache.go`
- [x] `internal/repository/redeem_code_repo.go`
- [x] `internal/repository/user_attribute_repo.go`

### Ent Schema 层

**完全删除**:
- [x] `ent/schema/redeem_code.go`
- [x] `ent/schema/promo_code.go`
- [x] `ent/schema/promo_code_usage.go`
- [x] `ent/schema/user_allowed_group.go`
- [x] `ent/schema/user_attribute_definition.go`
- [x] `ent/schema/user_attribute_value.go`

### Routes 层

**简化保留** (有 [LITE] 标记):
- [x] `internal/server/routes/auth.go` - 简化保留 (只保留 /login, /settings/public, /auth/me)
- [x] `internal/server/routes/user.go` - 简化保留 (只保留 API Key 和 Usage 管理路由)

## Phase 2: 修改 Schema 和创建数据库迁移 ✅ 完成

### Schema 修改
1. **api_key.go** - 添加 Lite 限额字段:
   - `daily_limit_usd`, `weekly_limit_usd`, `monthly_limit_usd`, `total_limit_usd`
   - `daily_usage_usd`, `weekly_usage_usd`, `monthly_usage_usd`, `total_usage_usd`
   - `usage_reset_daily`, `usage_reset_weekly`, `usage_reset_monthly`

2. **user.go** - 移除已删除的 edges:
   - `[LITE:DELETED] redeem_codes edge`
   - `[LITE:DELETED] allowed_groups edge`
   - `[LITE:DELETED] attribute_values edge`
   - `[LITE:DELETED] promo_code_usages edge`

3. **group.go** - 移除已删除的 edges:
   - `[LITE:DELETED] redeem_codes edge`
   - `[LITE:DELETED] allowed_users edge`

### 数据库迁移
- [x] 创建 `migrations/100_lite_mode.sql`

## Phase 3: Wire 依赖注入清理 ✅ 完成

### service/wire.go 修改
- [x] 移除 `NewAuthService`
- [x] 移除 `NewRedeemService`
- [x] 移除 `NewPromoService`
- [x] 移除 `NewEmailService`
- [x] 移除 `ProvideEmailQueueService`
- [x] 移除 `NewTurnstileService`
- [x] 移除 `NewIdentityService`
- [x] 移除 `NewUserAttributeService`

### repository/wire.go 修改
- [x] 移除 `NewRedeemCodeRepository`
- [x] 移除 `NewPromoCodeRepository`
- [x] 移除 `NewUserAttributeDefinitionRepository`
- [x] 移除 `NewUserAttributeValueRepository`
- [x] 移除 `NewEmailCache`
- [x] 移除 `NewIdentityCache`
- [x] 移除 `NewRedeemCache`
- [x] 移除 `NewTurnstileVerifier`

## Phase 4: Service 层依赖修复 ✅ 完成

### 错误定义迁移
- [x] `ErrInsufficientBalance` 迁移到 `billing_cache_service.go`
- [x] `ErrEmailExists` 迁移到 `user_service.go`
- [x] `ErrUserNotActive` 迁移到 `user_service.go`

### Service 修改
- [x] `gateway_service.go` - 移除 `identityService` 相关代码
- [x] `admin_service.go` - 移除 RedeemCode 相关接口和实现
- [x] `setting_handler.go` - 移除 EmailService 和 TurnstileService

## Phase 5: Ent 代码重新生成 ✅ 完成

- [x] 创建临时 stub 包 (`ent/ent.go`, `ent/intercept/intercept.go`)
- [x] 成功运行 `go generate ./ent/...`
- [x] Ent 代码成功重新生成

## Phase 6: 编译错误修复 ✅ 完成

### 已完成修复

**setting_handler.go**:
- [x] 移除 `service.EmailService` 引用
- [x] 移除 `service.TurnstileService` 引用
- [x] 移除 SMTP 测试方法

**repository/wire.go**:
- [x] 重构 `EntAndDB` 结构体 + Provider

**handler 层清理**:
- [x] 删除 `auth_handler.go`, `redeem_handler.go`
- [x] 删除 `auth_linuxdo_oauth.go`
- [x] 保留 `user_handler.go`, `api_key_handler.go`, `usage_handler.go` (Admin API Key 管理需要)
- [x] ~~删除 `subscription_handler.go`~~ (Admin 订阅管理需要，保留在 admin/ 目录)
- [x] 移除已删除的 Handler 引用 (`handler/handler.go`, `handler/wire.go`)

**routes 层清理**:
- [x] 删除 `routes/auth.go`
- [x] 保留 `routes/user.go` - 仅包含 API Key 管理路由 (移除 Redeem/Subscription)
- [x] 修改 `routes/admin.go` 移除 Redeem/Promo/User/UserAttribute 路由

**server 层清理**:
- [x] `server/http.go` 移除 jwtAuth 参数
- [x] `server/router.go` 移除 Auth 路由注册，保留 User 路由注册 (API Key 管理)

**CLI 清理**:
- [x] 删除 `cmd/jwtgen/` 整个目录
- [x] `cmd/server/wire.go` 移除 emailQueue 参数

**重新生成**:
- [x] `wire_gen.go` 成功重新生成
- [x] `go build ./...` 编译通过

## Phase 7: 核心业务逻辑 ✅ 完成

### 标准模式限额逻辑
- [x] 实现 `checkUsageLimits` 函数 - `billing_cache_service.go:checkAPIKeyLimitEligibility`
- [x] 实现 `resetExpiredUsage` 函数 - `api_key_usage_service.go:CheckAndResetExpiredPeriods`
- [x] 实现 `recordUsageLite` 函数 - `gateway_service.go:RecordUsage` (LITE 分支)
- [x] Gateway Usage 返回限额格式 - `gateway_handler.go:calculateAPIKeyRemaining`
- [x] Wire 依赖注入配置 - `repository/wire.go:ProvideAPIKeyUsageRepository`

### API Key 用量管理
- [x] 新增 `resetAPIKeyUsage` API - `admin/usage_handler.go:ResetAPIKeyUsage`
- [x] 注册路由 - `routes/admin.go:POST /api-keys/:id/reset-usage`
- [x] 更新 API Key DTO - `dto/types.go` 和 `dto/mappers.go`

## Phase 8: 前端适配 ✅ 完成

### 删除文件
- [x] 删除 `views/auth/` 目录
- [x] 删除 `views/user/` 目录
- [x] 删除 `views/admin/RedeemView.vue`
- [x] 删除 `views/admin/PromoCodesView.vue`
- [x] 删除 `views/admin/UsersView.vue`
- [x] 删除 `api/auth.ts`, `api/user.ts`, `api/redeem.ts`
- [x] 删除 `components/admin/user/*` - 用户管理组件
- [x] 删除 `components/user/profile/*` - 用户配置组件
- [x] 删除 `components/user/UserAttribute*.vue` - 用户属性组件
- [x] 删除 `components/user/dashboard/UserDashboardStats.vue`

### 修改文件
- [x] 修改 `router/index.ts` 移除相关路由
- [x] 修改 `AppSidebar.vue` 导航组件移除菜单项
- [x] 修改 `SettingsView.vue` 移除邮件/Turnstile/LinuxDo OAuth 配置
- [x] 修改 `stores/auth.ts` - 简化为 Admin API Key 认证
- [x] 修改 `stores/app.ts` - 使用 admin settings API
- [x] 修改 `stores/subscriptions.ts` - 使用 admin subscriptions API
- [x] 修改 `AuthLayout.vue` - 使用 admin settings API
- [x] 修改 `HomeView.vue` - 使用 admin settings API
- [x] 修改 `api/index.ts` - 修正 setupAPI 导出

### 新增功能
- [x] `types/index.ts` - API Key 限额字段 (daily/weekly/monthly/total_limit_usd, usage_usd)
- [x] `api/admin/settings.ts` - getPublicSettings/loginWithApiKey/getCurrentAdmin 方法
- [x] 前端编译测试通过

## Phase 9: 测试和验证 ✅ 完成

- [x] 后端编译通过
- [x] 前端编译通过
- [x] 运行时验证
- [x] 计费逻辑修复
- [x] 上游响应处理修复

### Phase 9 修复记录

**计费逻辑修复 - 余额检查移除** (2026-01-11 21:45):

问题：API 请求返回 `403 Insufficient account balance`，即使 Lite 模式应该不检查余额。

根本原因：
1. `billing_cache_service.go` - 当没有限额时回退到余额检查
2. `api_key_auth.go` 中间件 - 在请求进入 handler 前检查余额
3. `api_key_auth_google.go` 中间件 - 同上

修复内容：

*billing_cache_service.go*:
```go
// 修改前：回退到余额检查
return s.checkBalanceEligibility(ctx, apiKey.UserID)

// 修改后：无限制通过
// [LITE] 既没有 API Key 限额，也没有 Group 限额，无限制通过
return nil
```

*api_key_auth.go* (第 148-154 行):
```go
// 修改前：
} else {
    // 余额模式：检查用户余额
    if apiKey.User.Balance <= 0 {
        AbortWithError(c, 403, "INSUFFICIENT_BALANCE", "Insufficient account balance")
        return
    }
}

// 修改后：
}
// [LITE] 标准模式不在中间件检查余额，限额检查在 handler 中进行
```

*api_key_auth_google.go* (第 89-94 行):
```go
// 同样移除余额检查逻辑
```

**上游响应处理修复** (2026-01-11 21:50):

问题：当上游返回非 JSON 响应（如 HTML 错误页面）时，客户端收到空响应。

修复内容 - *gateway_service.go* `handleNonStreamingResponse`:
```go
if err := json.Unmarshal(body, &response); err != nil {
    // [LITE] 解析失败时仍然返回上游响应给客户端（可能是错误页面）
    responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.cfg.Security.ResponseHeaders)
    c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
    return nil, fmt.Errorf("parse response: %w", err)
}
```

**RunMode 引用清理** (2026-01-11):
- `billing_cache_service.go` - 移除 RunModeSimple 检查，直接执行 API Key 限额检查
- `gateway_service.go` - 移除 RunMode 条件，默认忽略分组限制
- `openai_gateway_service.go` - 同上
- `api_key_auth.go` / `api_key_auth_google.go` - 移除 RunMode 条件
- `main.go` - 移除启动时 RunMode 警告
- `config_test.go` - 删除 NormalizeRunMode 测试
- `gateway_multiplatform_test.go` - 移除测试中 RunMode 引用
- `api_key_auth_google_test.go` - 移除测试中 RunMode 引用

**API Key 管理功能恢复** (2026-01-11):
前端 API Key 管理页面需要保留给 Admin 使用，因此恢复了相关代码：

*前端恢复*:
- `frontend/src/api/keys.ts` - API Key API 模块
- `frontend/src/api/usage.ts` - 使用记录 API 模块
- `frontend/src/api/groups.ts` - 用户分组 API 模块（获取可用分组）
- `frontend/src/views/user/KeysView.vue` - API Key 管理页面
- `frontend/src/api/index.ts` - 添加 keysAPI, usageAPI, userGroupsAPI 导出
- `frontend/src/router/index.ts` - 添加 `/keys` 路由
- `frontend/src/components/layout/AppSidebar.vue` - 添加 "API 密钥" 菜单项

*后端恢复*:
- `internal/server/routes/user.go` - 用户路由（仅保留 API Key 管理，移除 Redeem/Subscription）
- `internal/handler/user_handler.go` - 用户 Handler
- `internal/handler/api_key_handler.go` - API Key Handler
- `internal/handler/usage_handler.go` - 使用记录 Handler

*后端修改*:
- `internal/handler/handler.go` - 添加 User, APIKey, Usage, Public 字段到 Handlers 结构体
- `internal/handler/wire.go` - 添加 NewUserHandler, NewAPIKeyHandler, NewUsageHandler 到 ProviderSet
- `internal/server/router.go` - 添加 RegisterUserRoutes 调用

**计费类型标签修改** (2026-01-11):
- `frontend/src/i18n/locales/zh.ts` - 标准模式标签从 "标准（余额）" 改为 "标准（限额）"
- 提示文字改为 "标准模式使用 API Key/分组限额控制。订阅模式使用配额限制。"

---

## 标记规范

所有删除或修改的代码都使用以下标记:
- `[LITE]` - 新增或修改的代码
- `[LITE:DELETED]` - 被删除功能的注释标记

## 下一步工作

1. ~~**Phase 7 核心**: 实现标准模式限额逻辑~~ ✅ 已完成

2. ~~**Phase 8 前端**: 前端清理和 UI 开发~~ ✅ 已完成

3. ~~**Phase 9 验证**: 运行时测试验证~~ ✅ 已完成

4. ~~**Phase 10: Git 仓库配置**~~ ✅ 已完成 (2026-01-11)
   - [x] Fork 上游仓库到 Kevin-Lab777/sub2api
   - [x] 配置 Git remote (origin=fork, upstream=原仓库)
   - [x] 创建 Light 分支
   - [x] 提交 Lite 核心修改 (182 files, +12689/-51010)
   - [x] 打版本标签 v0.1.46-lite
   - [x] 替换所有 GitHub URL (246 files)
   - [x] 推送到远程仓库

5. **Phase 11: 文档完善** (可选)
   - [ ] 更新 README.md 添加 Lite 版本说明
   - [ ] 更新 Docker 配置
   - [ ] 创建 GitHub Release

6. **Phase 12: 上游同步 v0.1.50** ✅ 已完成 (2026-01-12)
   - [x] `git fetch upstream && git merge upstream/main`
   - [x] 修复 import path (Wei-Shaw -> Kevin-Lab777)
   - [x] 处理上游新增 Ops 监控系统
   - [x] 删除依赖 EmailService 的文件 (ops_alert_evaluator, ops_scheduled_report)
   - [x] 移除 RunModeSimple 引用
   - [x] 添加 APIKeyUsageService 到 wire
   - [x] 重新生成 wire_gen.go
   - [x] 后端编译通过
   - [x] 前端编译通过 (修复 StreamTimeoutSettings 类型)
   - [x] 提交 merge commit
   - [x] 打标签 v0.1.50-lite
   - [x] 推送到远程仓库

7. **Phase 13: 上游同步 v0.1.76** ✅ 已完成 (2026-02-17)

   上游从 v0.1.53 → v0.1.76，跨 626 commits，808 files changed (+154k/-23k)

   ### 已完成 (全部)

   #### Step 1: Merge 并处理冲突 ✅
   - [x] `git fetch upstream && git merge upstream/main`
   - [x] 92 个冲突文件需要处理

   #### Step 2: 删除文件处理 (🔴 git rm) ✅
   已删除 26 个 Lite 不需要的冲突文件：
   - [x] `handler/auth_handler.go` → 后恢复（Lite 简化版存在于 HEAD）
   - [x] `handler/auth_linuxdo_oauth.go`
   - [x] `handler/admin/redeem_handler.go`
   - [x] `service/email_service.go`, `email_queue_service.go`
   - [x] `service/identity_service.go`
   - [x] `service/redeem_service.go`, `auth_service.go` → auth_service 后恢复
   - [x] `service/ops_alert_evaluator_service.go`, `ops_scheduled_report_service.go`
   - [x] `repository/email_cache.go`, `identity_cache.go`, `redeem_code_repo.go`
   - [x] `ent/schema/redeem_code.go`, `promo_code.go`
   - [x] `cmd/jwtgen/main.go`
   - [x] 前端：`RegisterView.vue`, `EmailVerifyView.vue`, `LinuxDoCallbackView.vue`
   - [x] 前端：`RedeemView.vue`(user+admin), `UsageView.vue`, `PromoCodesView.vue`, `UsersView.vue`
   - [x] 前端：`auth.ts`, `redeem.ts`, `UserDashboardCharts.vue`
   - [x] `config.yaml` (上游已删除)

   #### Step 3: 正常 Merge 文件 (🟢 accept theirs) ✅
   接受上游版本 28 个文件：
   - [x] README.md, README_CN.md, VERSION, go.mod
   - [x] admin handlers: account, ops_alerts, usage, gemini_v1beta
   - [x] repositories: api_key_repo, dashboard_aggregation, geminicli, http_upstream, openai_oauth, user_repo 等
   - [x] services: admin, antigravity_gateway, api_key, openai_oauth, ops_retry
   - [x] ent schemas: account, mixins/soft_delete, user_subscription
   - [x] 前端: App.vue, CreateAccountModal.vue, deploy/.env.example
   - [x] 测试: api_key_auth_google_test

   #### Step 4: Ent 生成文件 (accept theirs, 后重生成) ✅
   - [x] 接受上游 18 个 ent 生成文件 + wire_gen.go

   #### Step 5: Ent Schema 手动合并 (🟠) ✅
   - [x] `ent/schema/api_key.go` - 保留 Lite 限额字段 + 接受上游 quota/expires_at
   - [x] `ent/schema/group.go` - 保留删除的 edges + 接受上游新字段 (fallback_group, model_routing, sort_order)
   - [x] `ent/schema/user.go` - 保留删除的 edges + 接受上游 announcement_reads/TOTP 字段

   #### Step 6: 后端核心文件手动合并 (🟠) ✅
   - [x] `config/config.go` - 保留 RunModeLite + 接受上游 UsageCleanupConfig
   - [x] `handler/gateway_handler.go` - 保留 Lite Usage 端点 + 接受上游 domain/ctxkey
   - [x] `handler/handler.go` - 保留 Lite 简化 + 接受上游 ErrorPassthrough/Announcement/Totp
   - [x] `handler/wire.go` - 保留 Lite 简化 + 移除 SubscriptionHandler（Lite 不需要）
   - [x] `handler/dto/mappers.go` - 保留 Lite API Key 限额映射
   - [x] `repository/wire.go` - 保留 Lite 简化 + 接受上游 SessionLimitCache/Announcement repos
   - [x] `service/gateway_service.go` - 保留 Lite 用量记录 + 移除 identityService + 接受上游 claudeTokenProvider/sessionLimitCache
   - [x] `service/openai_gateway_service.go` - 接受上游 listSchedulableAccounts
   - [x] `service/wire.go` - 保留 Lite 删除 + 接受上游 UsageCleanup/TokenRefresh
   - [x] `routes/admin.go` - 保留 Lite 移除 + 接受上游 ErrorPassthrough 路由
   - [x] `routes/auth.go` - 保留 Lite 简化版 (checkout --ours)
   - [x] `routes/user.go` - 保留 Lite 简化版 (checkout --ours)

   #### Step 7: 前端手动合并 (🟠) ✅
   - [x] `api/index.ts` - 保留 Lite exports + 接受上游 totpAPI/announcementsAPI
   - [x] `AppHeader.vue` - 保留 today usage + 接受上游 AnnouncementBell
   - [x] `AppSidebar.vue` - 保留 Lite 简化导航 + Profile 入口
   - [x] `AuthLayout.vue` - 保留 Lite branding (Sub2API Lite)
   - [x] `router/index.ts` - 保留 Lite 简化路由
   - [x] `stores/auth.ts` - 保留 Lite Admin API Key auth + 接受上游 token refresh

   #### Step 8: Import 路径替换 ✅
   - [x] 替换所有 Go 文件中 `Wei-Shaw/sub2api` → `Kevin-Lab777/sub2api` (126 files)
   - [x] 替换 Markdown 文件中的 GitHub URL

   #### Step 9: 恢复误删文件 ✅
   - [x] 恢复 `handler/auth_handler.go` (Lite 简化版)
   - [x] 恢复 `service/auth_service.go` (Lite 简化版)

   #### Step 10: 上游新增文件依赖处理 ✅
   - [x] `service/totp_service.go` - 移除 EmailService/EmailQueueService 依赖，改为只使用密码验证
   - [x] `service/admin_service.go` - 移除所有 RedeemCode 引用
   - [x] `service/gateway_service.go` - 移除 Fingerprint/Identity 引用，添加本地 fingerprint struct
   - [x] `service/account_usage_service.go` - 移除 IdentityCache/Fingerprint
   - [x] `repository/claude_usage_service.go` - 移除 Fingerprint 引用
   - [x] `service/api_key.go` - 添加 Lite 限额方法 (HasAnyLimit, IsXxxLimitExceeded)
   - [x] 多个测试文件修复 (移除 redeem/identity 相关测试)

   #### Step 11: Ent 重新生成 ✅
   - [x] 删除所有 ent 生成文件（保留 schema/ 和 generate.go）
   - [x] 创建 intercept stub 用于 bootstrap
   - [x] `go generate ./ent/...` 成功
   - [x] 验证生成代码包含 Lite 限额字段 (DailyLimitUsd 等)
   - [x] 验证生成代码不包含已删除 schema (PromoCode/RedeemCode/UserAttribute)

   #### Step 12: 编译修复 ✅
   - [x] 移除 `userallowedgroup` 引用 - user_repo.go 全面清理（移除 AllowedGroups 同步、加载、过滤函数）
   - [x] 修复 repository/wire.go 语法错误（多余的 `}`）
   - [x] 添加 ResetAPIKeyUsage handler 到 admin/usage_handler.go
   - [x] 修复 usage_cleanup_handler_test.go NewUsageHandler 调用
   - [x] `go build ./...` 通过
   - [x] 前端修复: App.vue 移除 useSubscriptionStore 引用
   - [x] 前端修复: CreateAccountModal.vue 移除 isSimpleMode 引用
   - [x] 前端修复: 删除 ForgotPasswordView.vue、ResetPasswordView.vue
   - [x] 前端修复: 添加 TOTP 2FA 支持到 auth.ts 和 auth store
   - [x] 前端修复: 安装缺失的 npm 包 (marked, dompurify, qrcode, vue-draggable-plus)
   - [x] 前端编译通过

   #### Step 13: Wire 重新生成 ✅
   - [x] `GOTOOLCHAIN=go1.25.7 go run github.com/google/wire/cmd/wire@latest ./cmd/server/...`

   #### Step 14: 提交和标签 ⏳
   - [ ] 提交 merge commit
   - [ ] 打版本标签 v0.1.76-lite
   - [ ] 推送到远程仓库

   #### Step 15: 文档更新 ✅
   - [x] 更新 `sub2api-lite-progress.md`
   - [x] 更新 `sub2api-lite-changes.md`
   - [x] 更新 `MERGE_PLAYBOOK.md` 版本信息

   ### 上游新增功能（本次 merge 引入）
   - TOTP 双因素认证系统（已适配 Lite，移除邮件依赖）
   - 公告系统 (Announcement)
   - 错误透传规则 (ErrorPassthrough)
   - 使用清理服务 (UsageCleanup)
   - API Key quota/expires_at 字段
   - Group 新字段：fallback_group, model_routing, sort_order, mcp_xml_inject
   - Anthropic 差异化缓存计费
   - 前端 StatCard 溢出修复、侧边栏 Logo 闪烁修复
   - gosec CI 配置

8. **Phase 14: 上游同步 v0.1.83** ✅ 已完成 (2026-02-19)

   上游从 v0.1.76 → v0.1.83，跨 7 commits，539 files changed

   ### 已完成 (全部)

   #### Step 1: Merge 并处理冲突 ✅
   - [x] `git fetch upstream && git merge upstream/main`
   - [x] 仅 2 个冲突文件：VERSION (accept theirs) + UsageView.vue (git rm)

   #### Step 2: 删除文件处理 ✅
   - [x] `frontend/src/views/user/UsageView.vue` - git rm（Lite 已删除）
   - [x] 其他 Lite 已删除文件未被上游重新添加

   #### Step 3: 正常 Merge 文件 ✅
   - [x] 所有文件自动 merge 成功
   - [x] 验证 gateway_service.go Lite 标记完好
   - [x] 验证 dto/mappers.go Lite 标记完好
   - [x] 零 `Wei-Shaw/sub2api` import 路径泄漏

   #### Step 4: Ent 重新生成 ✅
   - [x] usage_log schema 新增 `cache_ttl_overridden` 字段
   - [x] `go generate ./ent/...` 成功

   #### Step 5: Wire 重新生成 ✅
   - [x] `GOTOOLCHAIN=go1.25.7 go run github.com/google/wire/cmd/wire@latest ./cmd/server/...`

   #### Step 6: 编译验证 ✅
   - [x] `go build ./...` 通过
   - [x] 前端 `pnpm build` 通过

   #### Step 7: 提交和标签 ⏳
   - [ ] 提交 merge commit
   - [ ] 打版本标签 v0.1.83-lite
   - [ ] 推送到远程仓库

   #### Step 8: 文档更新 ✅
   - [x] 更新 `sub2api-lite-progress.md`
   - [x] 更新 `sub2api-lite-changes.md`
   - [x] 更新 `MERGE_PLAYBOOK.md` 版本信息

   ### 上游新增功能（本次 merge 引入）
   - Cache TTL Override per account（管理员可强制替换账号缓存 TTL）
   - Codex 不支持参数清理 (strip unsupported params)
   - OpenAI x-stainless-* header CORS 修复
   - 前端 sonnet 4.6 快捷映射按钮
   - usage_log 新增 cache_ttl_overridden 字段
   - 新增迁移 055_add_cache_ttl_overridden.sql

9. **Phase 15: 上游同步 v0.1.88** ✅ 已完成 (2026-03-15)

   上游从 v0.1.83 → v0.1.88，856 files changed (+188k/-49k)，大规模重构

   ### 已完成

   #### Step 1: Merge 并处理冲突 ✅
   - [x] `git fetch upstream && git merge upstream/main`
   - [x] 所有冲突已自动解决（merge 状态：All conflicts fixed but you are still merging）

   #### Step 2: 删除文件处理 ✅
   - [x] `frontend/src/api/admin/redeem.ts` - git rm（上游新增但 Lite 不需要）
   - [x] `frontend/src/api/admin/promo.ts` - git rm（同上）
   - [x] `backend/internal/handler/admin/redeem_handler_test.go` - git rm（上游新增测试）
   - [x] `backend/internal/repository/redeem_cache_integration_test.go` - git rm
   - [x] `backend/internal/repository/redeem_cache_test.go` - git rm
   - [x] `backend/ent/schema/redeem_code.go` - 删除（上游 merge 恢复的 schema）
   - [x] `backend/ent/schema/promo_code.go` - 删除
   - [x] `backend/ent/schema/promo_code_usage.go` - 删除
   - [x] `backend/ent/schema/user_attribute_definition.go` - 删除
   - [x] `backend/ent/schema/user_attribute_value.go` - 删除

   #### Step 3: Import 路径替换 ✅
   - [x] 零 `Wei-Shaw/sub2api` import 路径泄漏（Go 源文件）
   - [x] ent/schema 文件中 Wei-Shaw 路径已全部替换为 Kevin-Lab777

   #### Step 4: Lite 修改验证 ✅
   - [x] 所有 11 项 Lite 核心修改保留完好（billing_cache, gateway, middleware 等）
   - [x] 上游新增文件无依赖已删除服务的问题

   #### Step 5: Ent Schema 修复 ✅
   - [x] `user.go` - 重新删除 redeem_codes/allowed_groups/attribute_values/promo_code_usages edges（被 merge 恢复）
   - [x] `group.go` - 重新删除 redeem_codes/allowed_users edges（被 merge 恢复）
   - [x] `api_key.go` - 重新添加 Lite 限额字段（daily/weekly/monthly/total_limit_usd 等，被 merge 覆盖）
   - [x] `user_allowed_group.go` - **决定保留**（上游已将其作为核心功能，AllowedGroups 被 admin_service/api_key_service/sora_quota_service 广泛使用）
   - [x] `account_group.go` + `user_allowed_group.go` - 修复 `field.ID()` composite key 注解导致 ent 生成失败的问题（改用 `field.Annotation{ID: ..., StructTag: ...}`）

   #### Step 6: Ent 重新生成 ✅
   - [x] 清理旧生成文件 + intercept stub
   - [x] `go generate ./ent/...` 成功
   - [x] 验证：无 Wei-Shaw 路径、无已删除 schema 类型、Lite 限额字段存在

   #### Step 7: 编译错误修复 ✅
   - [x] `admin_service.go` - 重新移除 RedeemCode 接口方法（ListRedeemCodes 等 6 个方法）
   - [x] `subscription_service.go` - 添加 `DefaultSubscriptionAssigner` 接口定义（上游使用但未定义）
   - [x] `api_key.go` (service) - 重新添加 Lite 限额字段 + HasAnyLimit/IsXxxLimitExceeded 方法
   - [x] `account_usage_service.go` - 添加本地 Fingerprint struct + IdentityCache interface（identity_service.go 已删除）
   - [x] `api_key_usage_repo.go` - 新建文件，实现 APIKeyUsageRepository 接口（IncrementUsage/ResetXxxUsage 等）
   - [x] `gateway_service.go:6994` - 修复 ResponseHeaders 类型（使用 responseHeaderFilter 而非 raw config）
   - [x] `gateway_service.go` + `openai_gateway_service.go` - 替换 `log.Printf` → `logger.LegacyPrintf`
   - [x] `openai_gateway_service.go` - 添加 `apiKeyUsageRepo` 字段到 OpenAIGatewayService struct
   - [x] `admin/usage_handler.go` - 添加 apiKeyUsageService 字段 + ResetAPIKeyUsage handler
   - [x] 测试文件参数修复（openai_gateway_record_usage_test, openai_ws_protocol_forward_test, usage_handler_request_type_test）

   #### Step 8: Wire 依赖注入修复 ✅
   - [x] 添加 `ProvideIdentityCache()` → 返回 nil（identity service 已删除）
   - [x] 添加 `NewSoraS3Storage` 到 service ProviderSet（上游新增）
   - [x] 添加 `NewSoraClientHandler` 到 handler ProviderSet（上游新增）
   - [x] 添加 `NewSoraGenerationService`, `NewSoraQuotaService` 到 service ProviderSet
   - [x] 添加 `NewSoraGenerationRepository` 到 repository ProviderSet

   #### Step 9: Wire 重新生成 ✅
   - [x] `GOTOOLCHAIN=go1.26.1 go run github.com/google/wire/cmd/wire@latest ./cmd/server/...`

   #### Step 10: 后端编译验证 ✅
   - [x] `go build ./...` 通过（零错误）

   #### Step 11: 前端编译验证 ✅
   - [x] `pnpm build` 验证前端编译
   - [x] 修复 `api/admin/index.ts` 移除 redeem/promo/userAttributes 导入
   - [x] 创建 `api/auth.ts` Lite 版（login/logout/getCurrentUser/isTotp2FARequired）
   - [x] 修复 `stores/app.ts` getPublicSettings 导入路径
   - [x] 修复 `stores/auth.ts` refreshToken/expires_in 类型兼容
   - [x] 修复 `SoraPromptBar.vue` 移除 subscriptionStore 依赖
   - [x] 修复 `AppSidebar.vue` 移除 unused RechargeSubscriptionIcon/SoraIcon
   - [x] 修复 `LoginView.vue` TotpLoginResponse 类型转换
   - [x] 修复 `UserEditModal.vue` 移除 userAttributes 调用
   - [x] 删除 `api/admin/userAttributes.ts`

   #### Step 12: VERSION 更新 ✅
   - [x] 更新 `backend/cmd/server/VERSION` 为 `0.1.88-Lite`

   #### Step 13: 数据库迁移修复 ✅
   - [x] 创建 `migrations/101_add_user_allowed_groups_id.sql` — 给 user_allowed_groups 表添加 id 自增列
   - [x] 原因：ent 生成代码默认查 `SELECT id, ...`，但原表用 (user_id, group_id) 复合主键无 id 列
   - [x] 本地测试环境已执行迁移

   #### Step 14: 提交和标签 ⏳ 待完成
   - [ ] 提交 merge commit
   - [ ] 打版本标签 v0.1.88-lite
   - [ ] 推送到远程仓库

   #### Step 15: 文档更新 ✅
   - [x] 更新 `sub2api-lite-progress.md`
   - [x] 更新 `sub2api-lite-changes.md`
   - [x] 更新 `sub2api-lite-dev-strategy.md` 版本历史
   - [x] 更新 `MERGE_PLAYBOOK.md` 版本信息

   ### 本次 Merge 重大决策

   1. **user_allowed_group.go 保留决策**：MERGE_PLAYBOOK 标记为删除，但上游已将其升级为核心功能（独立 join table + ent schema），被 admin_service/api_key_service/sora_quota_service/user_repo 广泛使用。决定保留 schema，仅删除 user.go/group.go 中的 edge shortcuts。
   2. **上游 API Key schema 重构**：上游新增 quota/rate_limit 体系（quota_used, rate_limit_5h/1d/7d），Lite 限额字段作为额外字段共存。
   3. **composite key 修复**：ent v0.14.5 的 `field.ID()` 注解与模板不兼容，改用 `field.Annotation{ID: ..., StructTag: map[string]string{}}` 解决。

   ### 上游新增功能（本次 merge 引入）
   - Sora 视频生成 SDK + 媒体存储 + S3 集成
   - API Key IP 白名单/黑名单
   - API Key quota/rate_limit 体系（5h/1d/7d 窗口）
   - API Key 过期时间 (expires_at)
   - 幂等性控制 (IdempotencyCoordinator)
   - 定时测试 (ScheduledTestService)
   - 用户消息串行队列 (UserMessageQueueService)
   - Scheduler Snapshot 服务
   - 延迟服务 (DeferredService)
   - Sora 配额管理 + Generation Service
   - Bedrock/AWS 支持
   - 生态项目文档

10. **Phase 16: Lite 新功能 — Active Hours 账号活跃时间段** ✅ 已完成 (2026-03-15)

    ### 功能说明
    控制账号仅在每天指定时间段内参与调度（如 8:00-22:00），支持跨午夜配置（如 22:00-06:00）。

    ### 实现方式
    - 利用现有 `extra` JSONB 字段存储 `active_hours` 规则，无需数据库迁移
    - 在 `IsSchedulable()` 方法末尾添加时间窗口检查
    - 使用项目 `timezone.Now()` 保持时区一致

    ### 改动文件
    - [x] `backend/internal/service/account.go` - 新增 ActiveHourRule/GetActiveHours/isInActiveWindow
    - [x] `backend/internal/handler/dto/types.go` - 新增 ActiveHourRule DTO
    - [x] `backend/internal/handler/dto/mappers.go` - 提取 active_hours 到 DTO 顶层
    - [x] `frontend/src/types/index.ts` - 新增 ActiveHourRule interface
    - [x] `frontend/src/components/account/EditAccountModal.vue` - 活跃时间段 UI
    - [x] `frontend/src/components/account/CreateAccountModal.vue` - 创建时支持配置
    - [x] `frontend/src/i18n/locales/zh.ts` - 中文翻译
    - [x] `frontend/src/i18n/locales/en.ts` - 英文翻译

    详细变更记录：`active-hours-changelog.md`

11. **Phase 17: 部署脚本修复与优化** ✅ 已完成 (2026-03-15)

    ### 修复内容
    - [x] `deploy/docker-deploy.sh` — GitHub URL 从上游 `Wei-Shaw/main` 改为 `Kevin-Lab777/Light`
    - [x] `deploy/docker-deploy.sh` — 下载 `docker-compose.yml` 而非已删除的 `docker-compose.local.yml`
    - [x] `deploy/docker-deploy.sh` — 数据目录改为 `data/sub2api`、`data/postgres_data`、`data/redis_data`
    - [x] 删除 `deploy/docker-compose.local.yml`（使用上游镜像 `weishaw/sub2api`，非 Lite 版）

    ### 优化内容
    - [x] `deploy/docker-compose.yml` — 支持通过 `.env` 切换 Docker named volumes 或本地目录映射
    - [x] 新增 `DATA_PATH`、`POSTGRES_DATA_PATH`、`REDIS_DATA_PATH` 环境变量
    - [x] 默认 Docker named volumes，设置路径后自动切换为 bind mount（方便迁移）
    - [x] `deploy/.env.example` — 新增 Data Storage 配置段

12. **Phase 18: 上游同步 v0.1.103** ✅ 已完成 (2026-03-28)

    上游从 v0.1.88 → v0.1.103，跨 116 commits (non-merge)

    ### 已完成

    #### Step 1: Merge 并处理冲突 ✅
    - [x] `git fetch upstream && git merge upstream/main`
    - [x] 所有冲突已解决（merge 状态：All conflicts fixed but you are still merging）

    #### Step 2: Import 路径替换 ✅
    - [x] 29 个 unstaged 文件需要处理
    - [x] 所有上游新增 Go 文件 `Wei-Shaw/sub2api` → `Kevin-Lab777/sub2api`
    - [x] 上游新增文件：backup_handler.go, endpoint.go, backend_mode_guard.go, antigravity_credits_overages.go 等

    #### Step 3: Lite 核心逻辑验证 ✅
    - [x] 所有 `[LITE]` 标记完好（billing_cache_service, gateway_service, api_key_auth, api_key_auth_google）
    - [x] 已删除的服务（email, turnstile, identity, redeem, promo, user_attribute）未被恢复到 wire 文件
    - [x] API Key 限额逻辑（checkAPIKeyLimitEligibility）完好
    - [x] Gateway RecordUsage 非订阅分支 Lite 逻辑完好

    #### Step 4: wire_gen.go Lite 重构 ✅
    - [x] 移除已删除服务（EmailService, TurnstileService, RedeemService, PromoService 等）
    - [x] 保留 APIKeyUsageRepository 注入
    - [x] 保留 ProvideIdentityCache (nil)
    - [x] 移除 soraGenerationService/soraQuotaService 的 wire 传递（Lite settingHandler 不依赖）
    - [x] 使用 Fork import 路径

    #### Step 5: VERSION 更新 ✅
    - [x] `0.1.88-Lite` → `0.1.103-Lite`

    #### Step 6: URL/品牌替换 ✅
    - [x] README.md / README_CN.md Star History URL
    - [x] Dockerfile.goreleaser image source label
    - [x] SettingsView.vue 文档链接
    - [x] KeyUsageView.vue GitHub 链接

    #### Step 7: 前端适配 ✅
    - [x] AppSidebar.vue 新增 DatabaseIcon（备份功能图标）
    - [x] gateway_service.go fingerprint struct 新增 UserAgent 字段

    #### Step 8: 文档更新 ✅
    - [x] 更新 `sub2api-lite-progress.md`
    - [x] 更新 `sub2api-lite-changes.md`
    - [x] 更新 `sub2api-lite-dev-strategy.md` 版本历史
    - [x] 更新 `MERGE_PLAYBOOK.md` 版本信息和 merge 历史
    - [x] 更新 `release-notes.md`

    #### Step 9: 提交 ⏳ 待完成
    - [ ] git add 所有 unstaged 文件
    - [ ] 提交 merge commit
    - [ ] 打版本标签 v0.1.103-lite
    - [ ] 推送到远程仓库

    ### 上游新增功能（本次 merge 引入）

    #### 备份与数据管理
    - 数据库定时备份与恢复（S3 兼容存储，支持 Cloudflare R2）
    - 备份和数据管理页面合并为设置页标签页

    #### Backend Mode & 设置
    - Backend Mode 切换（禁用用户自助服务）
    - 529 overload cooldown 开关和持续时间设置
    - config.yaml 只读挂载兼容

    #### 端点可观测性 & Dashboard
    - 统一端点标准化中间件 (endpoint normalization)
    - InboundEndpoint/UpstreamEndpoint 使用记录字段
    - upstream_model 列追踪（所有 gateway 路径）
    - Dashboard per-user drill-down（分组/模型/端点分布）
    - model_source 维度统计过滤
    - 使用表显示 upstream model 和端点分布图表

    #### 账号管理增强
    - 账号配额固定时间重置模式
    - 分组管理新增容量列（并发/会话/RPM 实时聚合）
    - 分组管理列表新增用量列与账号数分类
    - Group Capacity Service

    #### Antigravity & AI Credits
    - AI Credits 余额处理和 UI 展示增强
    - Antigravity 403 forbidden 状态检测和分类
    - Credits overages model key 稳定化
    - 统一 OAuth token refresh API + 分布式锁

    #### 平台 & 模型
    - Bedrock auth_mode credential 统一（bedrock-apikey → bedrock）
    - claude-haiku-4-5 variants 映射到 claude-sonnet-4-6
    - OpenAI model mapping (admin available models)
    - Claude output_config.effort 日志记录

    #### 订阅增强
    - 订阅管理页面教程指南弹窗
    - 订阅管理平台类型过滤

    #### Bug Fixes
    - 流式 failover 拼接腐化导致双 message_start
    - Bedrock 账户配额限制不生效
    - 批量编辑账号模型白名单不一致
    - Group quota limits 清除和 0 值处理
    - nil rate limit window 导致用量累积
    - DST-safe half-open date ranges
    - Ops aggregation PG overload 调优
    - native responses input ids 重写修复
    - OAuth 401 temp-unschedulable for Gemini 恢复

    ### 上游新增文件（Lite 直接接受）
    - `backend/internal/handler/admin/backup_handler.go`
    - `backend/internal/handler/endpoint.go`
    - `backend/internal/service/backup_service.go`
    - `backend/internal/service/group_capacity_service.go`
    - `backend/internal/service/metadata_userid.go`
    - `backend/internal/service/oauth_refresh_api.go`
    - `backend/internal/service/openai_model_mapping.go`
    - `backend/internal/service/refresh_policy.go`
    - `backend/internal/service/antigravity_credits_overages.go`
    - `backend/internal/service/usage_log_helpers.go`
    - `backend/internal/server/middleware/backend_mode_guard.go`
    - `backend/internal/repository/backup_pg_dumper.go`
    - `backend/internal/repository/backup_s3_store.go`
    - `backend/migrations/074_add_usage_log_endpoints.sql`
    - `backend/migrations/075_add_usage_log_upstream_model.sql`
    - `backend/migrations/075_map_haiku45_to_sonnet46.sql`
    - `backend/migrations/076_add_usage_log_upstream_model_index_notx.sql`
    - `frontend/src/api/admin/backup.ts`
    - `frontend/src/views/admin/BackupView.vue`
    - `frontend/src/components/common/GroupCapacityBadge.vue`
    - `frontend/src/components/charts/EndpointDistributionChart.vue`
    - `frontend/src/components/charts/UserBreakdownSubTable.vue`
    - 多个测试文件

13. **Phase 19: 上游同步 v0.1.109** ✅ 已完成 (2026-04-08)

    上游从 v0.1.103 → v0.1.109，跨 236 non-merge commits

    ### 已完成

    #### Step 1: Merge 并处理冲突 ✅
    - [x] `git fetch upstream && git merge upstream/main`
    - [x] 所有冲突已自动解决（merge 状态：All conflicts fixed but you are still merging）

    #### Step 2: 删除文件处理 ✅
    - [x] `backend/ent/schema/promo_code.go` - git rm（merge 恢复的 schema）
    - [x] `backend/ent/schema/promo_code_usage.go` - git rm
    - [x] `backend/ent/schema/redeem_code.go` - git rm
    - [x] `backend/ent/schema/user_attribute_definition.go` - git rm
    - [x] `backend/ent/schema/user_attribute_value.go` - git rm
    - [x] `backend/internal/service/identity_service_order_test.go` - git rm（上游新增测试，依赖已删除的 NewIdentityService）

    #### Step 3: Lite 核心逻辑验证 ✅
    - [x] ent/schema/user.go — 4 个 [LITE:DELETED] edge 完好
    - [x] ent/schema/group.go — 2 个 [LITE:DELETED] edge 完好
    - [x] ent/schema/api_key.go — 9 个 Lite 限额字段完好（+2 total 字段）
    - [x] billing_cache_service.go — checkAPIKeyLimitEligibility 直接 return，无 checkBalanceEligibility 调用 ✅
    - [x] gateway_service.go — RecordUsage 非订阅分支 Lite 限额逻辑完好 ✅
    - [x] gateway_service.go — listSchedulableAccounts mixed 分支无 RunModeSimple ✅
    - [x] api_key_auth.go — 无余额检查，有 [LITE] 标记 ✅
    - [x] api_key_auth_google.go — 无余额检查，有 [LITE] 标记 ✅

    #### Step 4: Import 路径替换 ✅
    - [x] `README_JA.md` — Wei-Shaw → Kevin-Lab777/Light（上游新增日语 README）
    - [x] 零 Wei-Shaw import 路径泄漏（Go 源文件）

    #### Step 5: Ent 重新生成 ✅
    - [x] 清理旧生成文件 + 创建 intercept stub（含 sql.Selector WhereP）
    - [x] `go generate ./ent/...` 成功（132 files）
    - [x] 验证：0 Wei-Shaw 引用、0 已删除 schema 类型、Lite 限额字段存在

    #### Step 6: Wire 依赖注入修复 ✅
    - [x] 修复 `cmd/server/wire.go` 语法错误（残留闭合括号，OpsSystemLogSink 后多余的 `}` `return nil` `}}`）
    - [x] 新增 `ProvideOAuthRefreshAPI` wrapper（`oauth_refresh_api.go`）— 解决 wire 无法处理 variadic `lockTTL ...time.Duration` 参数
    - [x] `service/wire.go` 中 `NewOAuthRefreshAPI` → `ProvideOAuthRefreshAPI`

    #### Step 7: Wire 重新生成 ✅
    - [x] `GOTOOLCHAIN=go1.26.1 go run github.com/google/wire/cmd/wire@latest ./cmd/server/...`
    - [x] wire_gen.go 中 0 已删除服务引用、APIKeyUsageRepository 注入完好

    #### Step 8: 后端编译验证 ✅
    - [x] `go build ./...` 通过（零错误）

    #### Step 9: 前端编译验证 ✅
    - [x] 安装缺失的 `@tanstack/vue-virtual`（上游新增虚拟滚动依赖）
    - [x] 修复 `router/index.ts` — 移除空路由对象（line 93，无 path 只有 meta）
    - [x] 修复 `router/index.ts` — 移除 DataManagementView 路由（view 文件不存在）
    - [x] 修复 `SettingsView.vue` — 注释掉 DataManagementSettings import（unused）
    - [x] 修复 `AppSidebar.vue` — 移除 data-management 导航项和 DatabaseIcon 组件
    - [x] `pnpm build` 通过

    #### Step 10: VERSION 更新 ✅
    - [x] `0.1.103-Lite` → `0.1.109-Lite`

    #### Step 11: Bugfix — OpenAI 账号调度分组查询 ✅ (2026-04-09)
    - [x] **openai_gateway_service.go** `listSchedulableAccounts` — 修复传 `nil` groupID 给 snapshot 导致走到 `ListSchedulableUngroupedByPlatform`，排除所有已分配分组的 OpenAI 账号
    - [x] **gateway_service.go** `listSchedulableAccounts` fallback 路径 — 改为尊重 groupID，有分组时查分组内账号
    - [x] 全面搜索所有 `[LITE]` 标记，确认无其他同类问题
    - [x] `go build ./...` 通过

    #### Step 12: 提交 ✅
    - [x] 提交 merge commit + bugfix
    - [x] 更新 v0.1.109-lite 标签
    - [x] 推送到远程仓库

    #### Step 12: 文档更新 ✅
    - [x] 更新 `sub2api-lite-progress.md`
    - [x] 更新 `sub2api-lite-changes.md`
    - [x] 更新 `sub2api-lite-dev-strategy.md` 版本历史
    - [x] 更新 `MERGE_PLAYBOOK.md` 版本信息和 merge 历史
    - [x] 更新 `release-notes.md`

    ### 本次 Merge 重大变化

    1. **Sora 平台完全移除（v0.1.107）**：上游在 v0.1.107 中 revert 并完全移除了 Sora 视频生成功能。相关 schema、handler、service、前端页面全部删除。Lite 直接受益——之前 merge 引入的 Sora 依赖也一并清理。
    2. **RecordUsage 重构合并**：上游将 `RecordUsage` 和 `RecordUsageWithLongContext` 合并为 `recordUsageCore` 共享核心。Lite 的非订阅分支逻辑（API Key 限额模式）仍在 `RecordUsage` 中保持完好。
    3. **Channel 定价系统**：上游新增 channel token pricing、model restriction、cache strategy 等完整 channel 管理功能。
    4. **OAuthRefreshAPI variadic 参数问题**：上游新增的 `NewOAuthRefreshAPI` 使用 variadic `lockTTL ...time.Duration`，wire v0.7.0 无法处理。创建 `ProvideOAuthRefreshAPI` wrapper 解决。
    5. **DataManagementView 清理**：上游在 Sora 移除过程中将 DataManagementView 功能合并到 BackupView。Lite 清理了残留的路由和导航引用。
    6. **OpenAI 账号调度 groupID 修复 (2026-04-09)**：`openai_gateway_service.go` 的 `listSchedulableAccounts` 传 `nil` 给 snapshot，导致 `loadAccountsFromDB` 走到 `ListSchedulableUngroupedByPlatform`——只返回**没分配分组**的账号。已分配到分组的 OpenAI 账号全部被排除，表现为 `no available OpenAI accounts`。修复为传真实 `groupID`。同步修复了 `gateway_service.go` 的 fallback 路径。

    ### 上游新增功能（本次 merge 引入）

    #### Channel 定价与管理
    - Channel token pricing 优先于 per-image billing
    - Channel model restriction 从 handler 移至 scheduling phase
    - Channel cache fail-close 策略 + 限制日志
    - Channel 定价 UI 重构（formatCacheTokens/formatMultiplier 提取）

    #### Sora 完全移除 (v0.1.107)
    - revert: completely remove all Sora functionality
    - 删除 Sora schema、handler、service、前端页面、S3 存储
    - DataManagementView 重组为 BackupView

    #### 模型支持
    - gpt-5.4-mini/nano 模型支持与定价配置
    - Beta 策略支持按模型区分处理（模型白名单）
    - max_claude_code_version 设置与禁用自动升级
    - claude-haiku-4-5 variants 映射更新

    #### 使用记录增强
    - Requested model 追踪（usage_log 新增 requested_model 字段）
    - 配额显示修复（每日/每周重置后不再显示过期累积用量）

    #### 前端优化
    - DataTable 虚拟滚动（@tanstack/vue-virtual）
    - 分页 pageSize 持久化到 localStorage
    - 用户管理新增分组列、分组筛选与专属分组一键替换
    - 批量编辑模型白名单修复

    #### Bug Fixes
    - 非流式路径上游终态事件 output 为空时从 delta 事件重建响应内容
    - 非 Claude Code 客户端 system prompt 迁移至 messages（绕过第三方应用检测）
    - billing mode display follows cost calculation result
    - channel_mapped override 不再错误回退 BillingModel
    - refresh token race condition 修复
    - Antigravity credits exhausted 429 正确标记
    - 流式 failover 记录原始 upstream status code

---

## 相关文档

| 文档 | 说明 |
|------|------|
| `sub2api-lite-tech-spec.md` | Lite 版本详细技术规格 |
| `sub2api-lite-changes.md` | 变更清单 |
| `MERGE_PLAYBOOK.md` | 上游合并操作手册 |
| `sub2api-lite-git-workflow.md` | Git 工作流记录 |
