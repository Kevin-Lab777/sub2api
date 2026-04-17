# Sub2API Lite - Merge Playbook

> **零上下文自包含文档** - AI 拿到此文档即可处理所有 merge 冲突
>
> 使用方式：`git fetch upstream && git merge upstream/main` 后，将此文档发送给 AI
>
> **当前版本**: v0.1.109-lite (同步上游 v0.1.109)
> **最后更新**: 2026-04-08
> **Light 分支**: https://github.com/Kevin-Lab777/sub2api/tree/Light

---

## Quick Reference Card

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         文件处理速查表                                    │
├─────────────────────────────────────────────────────────────────────────┤
│  🗑️  完全删除 (git rm)       │  这些文件在 Lite 中不存在                 │
│  ───────────────────────────┼───────────────────────────────────────────│
│  service/email_service.go   │  service/email_queue_service.go           │
│  service/turnstile_service  │  service/identity_service.go              │
│  service/redeem_*.go        │  service/promo_*.go                       │
│  service/user_attribute*.go │  service/ops_alert_evaluator_service.go   │
│  service/ops_scheduled_*    │  handler/redeem_handler.go                │
│  handler/admin/redeem_*     │  handler/admin/promo_handler.go           │
│  handler/admin/user_attr*   │  handler/auth_linuxdo_oauth*.go           │
│  handler/subscription_*     │  ent/schema/redeem_code.go                │
│  ent/schema/promo_*.go      │  ent/schema/user_attribute_*.go           │
│  ent/schema/user_allowed_*  │  ✅ KEPT (v0.1.88 升级为核心) + 101迁移    │
├─────────────────────────────────────────────────────────────────────────┤
│  📝  简化保留 (有 [LITE] 标记)  │  保留简化版，拒绝上游覆盖              │
│  ───────────────────────────┼───────────────────────────────────────────│
│  handler/auth_handler.go    │  只保留 Login + GetCurrentUser            │
│  handler/user_handler.go    │  保留 Profile 相关                        │
│  handler/api_key_handler.go │  保留完整 CRUD                            │
│  handler/usage_handler.go   │  保留完整功能                              │
│  service/auth_service.go    │  只保留 Login 和 ValidatePassword         │
│  routes/auth.go             │  只保留 /login + /settings/public         │
│  routes/user.go             │  保留 API Key + Usage 路由                 │
│  admin/user_handler.go      │  保留简化版 (无用户属性)                   │
├─────────────────────────────────────────────────────────────────────────┤
│  ✏️  保留 Lite 修改          │  有 [LITE] 标记，手动合并保留我们的逻辑   │
│  ───────────────────────────┼───────────────────────────────────────────│
│  config/config.go           │  移除 RunModeSimple (只有 standard)        │
│  billing_cache_service.go   │  checkAPIKeyLimitEligibility 限额检查     │
│  gateway_service.go         │  RecordUsage 不扣余额                      │
│  gateway_handler.go         │  Usage 返回限额信息                        │
│  routes/admin.go            │  跳过已删除模块的路由                       │
│  ent/schema/api_key.go      │  限额字段 (daily/weekly/monthly_limit)     │
│  ent/schema/user.go         │  移除已删除 schema 的 Edge                 │
│  ent/schema/group.go        │  移除 redeem_codes/allowed_users Edge      │
│  service/wire.go            │  移除已删除服务的 Provider                  │
│  cmd/server/wire.go         │  移除已删除服务的 cleanup                   │
│  handler/handler.go         │  Handlers 结构体字段                       │
│  handler/wire.go            │  Handler ProviderSet                       │
│  middleware/api_key_auth*   │  移除余额检查逻辑                           │
├─────────────────────────────────────────────────────────────────────────┤
│  ✅  正常 Merge              │  接受上游更新                              │
│  ───────────────────────────┼───────────────────────────────────────────│
│  **/account*.go             │  **/proxy*.go                             │
│  **/group_handler.go        │  **/dashboard*.go                         │
│  **/setting*.go             │  **/system*.go                            │
│  **/*_oauth*.go (账号级)    │  **/billing_service.go                    │
│  **/pricing*.go             │  **/middleware/** (除 api_key_auth)       │
│  **/pkg/**                  │  migrations/** (我们的从100开始)          │
│  admin/usage_handler.go     │  admin/subscription_handler.go            │
│  **/ops_*.go (新增监控)     │  frontend/** (需检查)                     │
│  Dockerfile                 │  docker-compose.yml (保留 Lite 版)        │
│  docker-compose.local.yml   │  ❌ 已删除 (使用上游镜像)                  │
└─────────────────────────────────────────────────────────────────────────┘
```

### 上游新增功能处理

```
┌─────────────────────────────────────────────────────────────────────────┐
│  上游新增文件判断规则                                                     │
├─────────────────────────────────────────────────────────────────────────┤
│  检查依赖                   │  处理方式                                   │
│  ───────────────────────────┼───────────────────────────────────────────│
│  依赖 EmailService          │  删除该文件或移除依赖                       │
│  依赖 TurnstileService      │  删除该文件或移除依赖                       │
│  依赖 IdentityService       │  删除该文件或移除依赖                       │
│  依赖 RedeemService         │  删除该文件                                 │
│  依赖 PromoService          │  删除该文件                                 │
│  依赖 UserAttributeService  │  删除该文件                                 │
│  引用 config.RunModeSimple  │  移除该条件判断                             │
│  无以上依赖                 │  正常接受                                   │
└─────────────────────────────────────────────────────────────────────────┘
```

### Frontend 速查表

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         前端文件处理速查表                                │
├─────────────────────────────────────────────────────────────────────────┤
│  🗑️  删除整个目录                                                         │
│  ───────────────────────────────────────────────────────────────────────│
│  src/views/auth/**          │  认证页面 (Register, OAuth 等，保留 Login) │
├─────────────────────────────────────────────────────────────────────────┤
│  🗑️  删除单个文件                                                         │
│  ───────────────────────────────────────────────────────────────────────│
│  views/admin/RedeemView.vue       │  兑换码管理                          │
│  views/admin/PromoCodesView.vue   │  优惠码管理                          │
│  views/admin/UsersView.vue        │  用户管理                            │
│  api/redeem.ts                    │  兑换码 API                          │
├─────────────────────────────────────────────────────────────────────────┤
│  ✏️  修改 (保留 Lite 版本)                                                │
│  ───────────────────────────────────────────────────────────────────────│
│  router/index.ts            │  保留 /profile 路由，移除其他 user 路由    │
│  views/admin/SettingsView   │  删除注册/邀请码/优惠码/Turnstile/LinuxDO │
│                             │  保留邮箱验证/密码重置/TOTP 2FA           │
│  AppSidebar.vue             │  底部添加 Profile 入口                    │
├─────────────────────────────────────────────────────────────────────────┤
│  ✅  保留 (Lite 恢复)                                                     │
│  ───────────────────────────────────────────────────────────────────────│
│  api/user.ts                │  用户 API (Profile, ChangePassword)       │
│  views/user/ProfileView.vue │  个人资料页面                              │
│  components/user/profile/*  │  Profile 相关组件                         │
├─────────────────────────────────────────────────────────────────────────┤
│  ➕  新增/增强                                                            │
│  ───────────────────────────────────────────────────────────────────────│
│  API Key 管理 UI            │  添加限额字段、用量显示、重置按钮           │
│  api/admin/keys.ts          │  resetAPIKeyUsage() 方法                  │
│  i18n/locales/zh.ts         │  分组计费类型: "Lite 模式（无余额限制）"    │
│  i18n/locales/en.ts         │  "Lite Mode (No Balance Limit)"           │
│  GroupsView.vue             │  standard 分组也可设限额字段               │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Part 1: 完全删除的文件清单

> 这些文件在 Lite 版本已完全删除，上游更新时直接 `git rm`

### 1.1 邮件/验证服务 (完全删除)

```bash
backend/internal/service/email_service.go
backend/internal/service/email_queue_service.go
backend/internal/service/turnstile_service.go
backend/internal/service/identity_service.go
```

### 1.2 兑换码系统 (完全删除)

```bash
# Handler
backend/internal/handler/redeem_handler.go
backend/internal/handler/admin/redeem_handler.go

# Service
backend/internal/service/redeem_service.go
backend/internal/service/redeem_code.go

# Schema
backend/ent/schema/redeem_code.go
```

### 1.3 优惠码系统 (完全删除)

```bash
# Handler
backend/internal/handler/admin/promo_handler.go

# Service
backend/internal/service/promo_service.go
backend/internal/service/promo_code.go
backend/internal/service/promo_code_repository.go

# Schema
backend/ent/schema/promo_code.go
backend/ent/schema/promo_code_usage.go
```

### 1.4 用户属性系统 (完全删除)

```bash
# Handler
backend/internal/handler/admin/user_attribute_handler.go

# Service
backend/internal/service/user_attribute.go
backend/internal/service/user_attribute_service.go

# Schema
backend/ent/schema/user_attribute_definition.go
backend/ent/schema/user_attribute_value.go
backend/ent/schema/user_allowed_group.go
```

### 1.5 其他删除

```bash
# LinuxDO OAuth
backend/internal/handler/auth_linuxdo_oauth.go
backend/internal/handler/auth_linuxdo_oauth_test.go

# 用户端订阅 Handler (由 Admin 统一管理)
backend/internal/handler/subscription_handler.go

# Ops 依赖 EmailService 的服务
backend/internal/service/ops_alert_evaluator_service.go
backend/internal/service/ops_scheduled_report_service.go
```

### 1.6 前端删除清单

```bash
# 认证页面 - 删除部分 (保留 LoginView.vue)
frontend/src/views/auth/RegisterView.vue
frontend/src/views/auth/EmailVerifyView.vue
frontend/src/views/auth/LinuxDoCallbackView.vue
frontend/src/views/auth/OAuthCallbackView.vue

# 用户端页面 - 删除部分 (保留 ProfileView.vue, KeysView.vue)
frontend/src/views/user/DashboardView.vue
frontend/src/views/user/RedeemView.vue
frontend/src/views/user/SubscriptionsView.vue
frontend/src/views/user/UsageView.vue

# Admin 删除页面
frontend/src/views/admin/RedeemView.vue
frontend/src/views/admin/PromoCodesView.vue
frontend/src/views/admin/UsersView.vue

# API 文件删除
frontend/src/api/redeem.ts
```

### 1.7 前端保留清单 (Lite 恢复)

```bash
# 用户 Profile 功能 - 保留
frontend/src/api/user.ts
frontend/src/views/user/ProfileView.vue
frontend/src/views/user/KeysView.vue
frontend/src/components/user/profile/ProfileInfoCard.vue
frontend/src/components/user/profile/ProfileEditForm.vue
frontend/src/components/user/profile/ProfilePasswordForm.vue
```

---

## Part 1.5: 简化保留的文件清单

> 这些文件保留但大幅简化，有 [LITE] 标记，merge 时保留 Lite 版本

### 1.5.1 Handler 层简化

| 文件 | 保留功能 | 删除功能 |
|------|----------|----------|
| `handler/auth_handler.go` | Login, GetCurrentUser | Register, SendVerifyCode, ValidatePromoCode |
| `handler/user_handler.go` | GetProfile, ChangePassword, UpdateProfile | - |
| `handler/api_key_handler.go` | 完整 CRUD | - |
| `handler/usage_handler.go` | 完整功能 | - |
| `admin/user_handler.go` | 用户 CRUD, 余额操作 | 用户属性相关 |

### 1.5.2 Service 层简化

| 文件 | 保留功能 | 删除功能 |
|------|----------|----------|
| `service/auth_service.go` | Login, ValidatePassword, GetUserByID | Register, SendVerifyCode, ValidatePromoCode |

### 1.5.3 Routes 层简化

| 文件 | 保留路由 | 删除路由 |
|------|----------|----------|
| `routes/auth.go` | /login, /settings/public, /auth/me | /register, /send-verify-code, /oauth/* |
| `routes/user.go` | /keys/*, /usage/*, /groups/available, /user/totp/* | /redeem/*, /subscriptions/* |

---

## Part 2: Lite 代码块 (完整版本)

> **重要**：实现代码后，将每个文件的完整 Lite 代码填入此处

### 2.1 config/config.go

**位置**: `backend/internal/config/config.go`

**Lite 新增常量**:
```go
// [LITE:ADD] 在 RunMode 常量区域添加
const (
    RunModeStandard = "standard"
    RunModeSimple   = "simple"
    RunModeLite     = "lite"  // [LITE:ADD]
)
```

**Lite 修改的函数**:
```go
// [LITE:MODIFY] NormalizeRunMode 函数
func NormalizeRunMode(value string) string {
    switch strings.ToLower(strings.TrimSpace(value)) {
    case "standard", "":
        return RunModeStandard
    case "simple":
        return RunModeSimple
    case "lite":           // [LITE:ADD]
        return RunModeLite // [LITE:ADD]
    default:
        return RunModeStandard
    }
}
```

---

### 2.2 billing_cache_service.go

**位置**: `backend/internal/service/billing_cache_service.go`

**⚠️ v0.1.88 merge 教训**: 上游在 `CheckBillingEligibility` 中新增了 `checkBalanceEligibility`（余额检查）和 `checkAPIKeyRateLimits`（API Key 速率限制），把 Lite 的 `return` 改成了 `if err`，导致余额检查挡在 Lite 限额检查前面。必须保持 Lite 的直接 return 模式。

**Lite 修改的函数** - CheckBillingEligibility:
```go
// [LITE:MODIFY] CheckBillingEligibility
// 关键：非订阅模式直接 return checkAPIKeyLimitEligibility，不检查余额
func (s *BillingCacheService) CheckBillingEligibility(ctx context.Context, user *User, apiKey *APIKey, group *Group, subscription *UserSubscription) error {
    // [LITE] 直接检查 API Key 限额，不再有简易模式跳过
    if s.circuitBreaker != nil && !s.circuitBreaker.Allow() {
        return ErrBillingServiceUnavailable
    }

    // 判断计费模式
    isSubscriptionMode := group != nil && group.IsSubscriptionType() && subscription != nil

    if isSubscriptionMode {
        return s.checkSubscriptionEligibility(ctx, user.ID, group, subscription)
    }

    // [LITE] 标准分组模式：检查 API Key 限额
    // 优先使用 API Key 自身限额，如果没有设置则使用 Group 限额
    if apiKey != nil {
        return s.checkAPIKeyLimitEligibility(ctx, apiKey, group)
    }

    // [LITE] 没有 API Key 时，无限制通过（不检查余额）
    return nil
}
```

**⚠️ 上游可能插入的代码（必须删除）**:
```go
// ❌ 以下代码是上游的，Lite 必须删除：
} else {
    if err := s.checkBalanceEligibility(ctx, user.ID); err != nil {  // ❌ 删除
        return err
    }
}
// Check API Key rate limits (applies to both billing modes)       // ❌ 删除
if apiKey != nil && apiKey.HasRateLimits() {                        // ❌ 删除
    if err := s.checkAPIKeyRateLimits(ctx, apiKey); err != nil {    // ❌ 删除
        return err                                                  // ❌ 删除
    }                                                               // ❌ 删除
}                                                                   // ❌ 删除
```

**Lite 函数** - checkAPIKeyLimitEligibility (完整):
```go
// [LITE] checkAPIKeyLimitEligibility 检查 API Key 限额
// 优先级：API Key 限额 > Group 限额 > 无限制通过
func (s *BillingCacheService) checkAPIKeyLimitEligibility(ctx context.Context, apiKey *APIKey, group *Group) error {
    // 检查 API Key 自身限额
    if apiKey.HasAnyLimit() {
        if apiKey.IsDailyLimitExceeded() {
            return ErrAPIKeyDailyLimitExceeded
        }
        if apiKey.IsWeeklyLimitExceeded() {
            return ErrAPIKeyWeeklyLimitExceeded
        }
        if apiKey.IsMonthlyLimitExceeded() {
            return ErrAPIKeyMonthlyLimitExceeded
        }
        if apiKey.IsTotalLimitExceeded() {
            return ErrAPIKeyTotalLimitExceeded
        }
        return nil
    }

    // API Key 没有设置限额，检查 Group 限额
    if group != nil {
        if group.HasDailyLimit() && apiKey.DailyUsageUSD >= *group.DailyLimitUSD {
            return ErrAPIKeyDailyLimitExceeded
        }
        if group.HasWeeklyLimit() && apiKey.WeeklyUsageUSD >= *group.WeeklyLimitUSD {
            return ErrAPIKeyWeeklyLimitExceeded
        }
        if group.HasMonthlyLimit() && apiKey.MonthlyUsageUSD >= *group.MonthlyLimitUSD {
            return ErrAPIKeyMonthlyLimitExceeded
        }
        if group.HasDailyLimit() || group.HasWeeklyLimit() || group.HasMonthlyLimit() {
            return nil
        }
    }

    // [LITE] 既没有 API Key 限额，也没有 Group 限额，无限制通过
    // 不再回退到余额检查
    return nil
}
```

---

### 2.3 gateway_service.go

**位置**: `backend/internal/service/gateway_service.go`

**⚠️ v0.1.88 merge 教训**: 上游在 `listSchedulableAccounts` 的 mixed 分支新增了 `RunModeSimple` 条件判断，导致 Lite 模式走到 `ListSchedulableUngroupedByPlatforms`，只能拿到无分组账号。Lite 必须保持无 groupID 时查所有账号。

**Lite 修改的函数** - listSchedulableAccounts (mixed 分支):
```go
// [LITE:MODIFY] listSchedulableAccounts - mixed 分支
// 关键：无 groupID 时直接查所有平台账号，不区分 RunMode
if useMixed {
    platforms := []string{platform, PlatformAntigravity}
    var accounts []Account
    var err error
    if groupID != nil {
        accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatforms(ctx, *groupID, platforms)
    } else {
        accounts, err = s.accountRepo.ListSchedulableByPlatforms(ctx, platforms)  // [LITE] 查所有
    }
    // ...
}
```

**⚠️ 上游可能插入的代码（必须删除）**:
```go
// ❌ 以下条件分支是上游的，Lite 必须删除：
} else if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {   // ❌ 删除
    accounts, err = s.accountRepo.ListSchedulableByPlatforms(...)    // ❌ 删除
} else {                                                             // ❌ 删除
    accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatforms(...)  // ❌ 删除
}
// ✅ 正确写法：
} else {
    accounts, err = s.accountRepo.ListSchedulableByPlatforms(ctx, platforms)
}
```

**Lite 修改的函数** - RecordUsage (非订阅分支):
```go
// [LITE:MODIFY] RecordUsage - 非订阅分支
// API Key 限额模式：如果 API Key 或 Group 有限额，记录用量而不扣余额
} else {
    hasAPIKeyLimits := apiKey != nil && apiKey.HasAnyLimit()
    hasGroupLimits := apiKey != nil && apiKey.Group != nil &&
        (apiKey.Group.HasDailyLimit() || apiKey.Group.HasWeeklyLimit() || apiKey.Group.HasMonthlyLimit())

    if hasAPIKeyLimits || hasGroupLimits {
        // 限额模式：更新 API Key 用量（使用 ActualCost 考虑倍率后的费用）
        if shouldBill && cost.ActualCost > 0 && s.apiKeyUsageRepo != nil {
            if err := s.apiKeyUsageRepo.IncrementUsage(ctx, apiKey.ID, cost.ActualCost); err != nil {
                logger.LegacyPrintf("service.gateway", "Increment API key usage failed: %v", err)
            }
        }
    } else {
        // 余额模式：扣除用户余额
        if shouldBill && cost.ActualCost > 0 {
            if err := s.userRepo.DeductBalance(ctx, user.ID, cost.ActualCost); err != nil {
                logger.LegacyPrintf("service.gateway", "Deduct balance failed: %v", err)
            }
            s.billingCacheService.QueueDeductBalance(user.ID, cost.ActualCost)
        }
    }
}
```

---

### 2.4 gateway_handler.go

**位置**: `backend/internal/handler/gateway_handler.go`

**Lite 修改的函数** - Usage:
```go
// [LITE:MODIFY] Usage endpoint
func (h *GatewayHandler) Usage(c *gin.Context) {
    apiKey := c.MustGet("apiKey").(*ent.APIKey)

    // [LITE] Lite 模式返回限额信息
    if h.cfg.RunMode == config.RunModeLite {
        c.JSON(http.StatusOK, gin.H{
            "object": "usage",
            "daily": gin.H{
                "limit_usd": apiKey.DailyLimitUsd,
                "used_usd":  apiKey.DailyUsageUsd,
                "reset_at":  apiKey.UsageResetDaily,
            },
            "weekly": gin.H{
                "limit_usd": apiKey.WeeklyLimitUsd,
                "used_usd":  apiKey.WeeklyUsageUsd,
                "reset_at":  apiKey.UsageResetWeekly,
            },
            "monthly": gin.H{
                "limit_usd": apiKey.MonthlyLimitUsd,
                "used_usd":  apiKey.MonthlyUsageUsd,
                "reset_at":  apiKey.UsageResetMonthly,
            },
        })
        return
    }

    // === 以下是原有标准模式逻辑，保持不变 ===
    // ... 原有代码 ...
}
```

---

### 2.5 ent/schema/api_key.go

**位置**: `backend/ent/schema/api_key.go`

**Lite 新增字段** (在 Fields() 函数中添加):
```go
// [LITE:ADD] 限额字段
field.Float("daily_limit_usd").Optional().Nillable().
    Comment("Daily usage limit in USD"),
field.Float("weekly_limit_usd").Optional().Nillable().
    Comment("Weekly usage limit in USD"),
field.Float("monthly_limit_usd").Optional().Nillable().
    Comment("Monthly usage limit in USD"),

// [LITE:ADD] 用量追踪字段
field.Float("daily_usage_usd").Default(0).
    Comment("Current daily usage in USD"),
field.Float("weekly_usage_usd").Default(0).
    Comment("Current weekly usage in USD"),
field.Float("monthly_usage_usd").Default(0).
    Comment("Current monthly usage in USD"),

// [LITE:ADD] 重置时间字段
field.Time("usage_reset_daily").Optional().Nillable().
    Comment("Next daily reset time"),
field.Time("usage_reset_weekly").Optional().Nillable().
    Comment("Next weekly reset time"),
field.Time("usage_reset_monthly").Optional().Nillable().
    Comment("Next monthly reset time"),
```

---

### 2.6 ent/schema/user.go

**位置**: `backend/ent/schema/user.go`

**Lite 删除的 Edge** (删除以下边关系):
```go
// [LITE:DELETE] 删除以下 Edge
edge.To("redeem_codes", RedeemCode.Type),
edge.To("promo_usages", PromoCodeUsage.Type),
edge.To("attribute_values", UserAttributeValue.Type),
edge.To("allowed_groups", UserAllowedGroup.Type),
```

---

### 2.7 routes/admin.go

**位置**: `backend/internal/server/routes/admin.go`

**Lite 跳过的路由注册**:
```go
func RegisterAdminRoutes(r *gin.RouterGroup, h *handler.Handler, cfg *config.Config) {
    admin := r.Group("/admin")
    admin.Use(middleware.AdminAuth(cfg))

    // 保留的路由
    RegisterAccountRoutes(admin, h)
    RegisterProxyRoutes(admin, h)
    RegisterGroupRoutes(admin, h)
    RegisterAPIKeyRoutes(admin, h)
    RegisterUsageRoutes(admin, h)
    RegisterDashboardRoutes(admin, h)
    RegisterSettingRoutes(admin, h)
    RegisterSystemRoutes(admin, h)
    RegisterSubscriptionRoutes(admin, h)

    // [LITE:SKIP] 以下路由在 Lite 模式不注册
    if cfg.RunMode != config.RunModeLite {
        RegisterRedeemRoutes(admin, h)
        RegisterPromoRoutes(admin, h)
        RegisterUserAttributeRoutes(admin, h)
        RegisterUserRoutes(admin, h)
    }
}
```

---

### 2.8 handler/handler.go

**位置**: `backend/internal/handler/handler.go`

**Lite 移除的 Handler 引用**:
```go
// [LITE:DELETE] 移除以下字段
type Handler struct {
    // 保留
    Gateway        *GatewayHandler
    Account        *admin.AccountHandler
    Proxy          *admin.ProxyHandler
    Group          *admin.GroupHandler
    APIKey         *admin.APIKeyHandler
    Usage          *admin.UsageHandler
    Dashboard      *admin.DashboardHandler
    Setting        *admin.SettingHandler
    System         *admin.SystemHandler
    Subscription   *admin.SubscriptionHandler

    // [LITE:DELETE] 删除以下
    // Auth           *AuthHandler
    // User           *UserHandler
    // Redeem         *RedeemHandler
    // PromoHandler   *admin.PromoHandler
    // UserAttribute  *admin.UserAttributeHandler
}
```

---

### 2.9 Lite 新增文件

#### migrations/100_lite_mode.sql

```sql
-- [LITE:ADD] Lite 模式数据库迁移

-- API Key 限额字段
ALTER TABLE api_keys ADD COLUMN daily_limit_usd REAL;
ALTER TABLE api_keys ADD COLUMN weekly_limit_usd REAL;
ALTER TABLE api_keys ADD COLUMN monthly_limit_usd REAL;

-- API Key 用量追踪字段
ALTER TABLE api_keys ADD COLUMN daily_usage_usd REAL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN weekly_usage_usd REAL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN monthly_usage_usd REAL DEFAULT 0;

-- API Key 重置时间字段
ALTER TABLE api_keys ADD COLUMN usage_reset_daily DATETIME;
ALTER TABLE api_keys ADD COLUMN usage_reset_weekly DATETIME;
ALTER TABLE api_keys ADD COLUMN usage_reset_monthly DATETIME;
```

---

## Part 3: 场景处理规则

### 3.1 上游新增文件

```
规则：根据文件路径模式判断

新增文件匹配以下模式 → 直接删除 (git rm)
├── **/auth*.go
├── **/user_handler.go, **/user_attribute*.go
├── **/redeem*.go
├── **/promo*.go
├── **/identity*.go
├── **/turnstile*.go
├── **/email_*.go (除了通知类)
└── **/user_allowed*.go

新增 ent/schema/*.go → 检查是否关联删除的系统
├── 关联 redeem/promo/user_attribute → 删除
└── 其他 → 接受

其他新增文件 → 默认接受
```

### 3.2 双方都修改了同一个文件

```
规则：按文件分类处理

config.go, billing_cache_service.go, gateway_*.go:
  1. 保留 [LITE] 标记的代码块
  2. 接受上游对其他部分的更新
  3. 手动合并，确保两边的改动都保留

wire.go, handler.go:
  1. 保留我们删除 Provider/Handler 的修改
  2. 接受上游新增的其他 Provider/Handler

routes/admin.go:
  1. 保留 [LITE:SKIP] 条件判断
  2. 接受上游新增的路由

ent/schema/*.go:
  1. 保留我们新增的字段
  2. 接受上游新增的字段/索引
  3. 检查 Edge 是否引用了删除的 schema
```

### 3.3 上游删除了文件

```
规则：

上游删除的文件本来就在我们的删除清单 → 无需处理
上游删除的文件我们保留着 → 检查是否影响编译，按需处理
```

### 3.4 Ent Schema Edge 冲突

```
规则：删除 schema 后 Edge 引用报错

1. 找到报错的 schema 文件
2. 在 Edges() 函数中删除引用已删除 schema 的 edge
3. 重新生成 ent: go generate ./ent
```

---

## Part 4: 执行流程

### Step 1: 查看冲突状态

```bash
# 查看所有冲突文件
git diff --name-only --diff-filter=U

# 查看未跟踪的新文件 (上游新增)
git status --porcelain | grep "^??"
```

### Step 2: 处理删除文件

```bash
# 对于 Part 1 中列出的所有文件，如果出现在冲突中
git rm <file>

# 批量删除 (如果文件存在)
for f in \
  backend/internal/handler/auth_handler.go \
  backend/internal/handler/auth_linuxdo_oauth.go \
  backend/internal/handler/user_handler.go \
  backend/internal/handler/redeem_handler.go \
  backend/internal/handler/admin/redeem_handler.go \
  backend/internal/handler/admin/promo_handler.go \
  backend/internal/handler/admin/user_attribute_handler.go \
  backend/internal/handler/admin/user_handler.go \
  backend/internal/service/auth_service.go \
  backend/ent/schema/redeem_code.go \
  backend/ent/schema/promo_code.go \
  backend/ent/schema/promo_code_usage.go \
  backend/ent/schema/user_attribute_definition.go \
  backend/ent/schema/user_attribute_value.go \
  backend/ent/schema/user_allowed_group.go \
  backend/internal/server/routes/auth.go \
  backend/internal/server/routes/user.go
do
  [ -f "$f" ] && git rm "$f"
done
```

### Step 3: 处理保留 Lite 修改的文件

对于每个有 Lite 修改的文件：

```bash
# 方式1: 如果上游没改这个文件，保留我们的
git checkout --ours <file>

# 方式2: 如果双方都改了，手动合并
# 打开文件，对照 Part 2 中的代码，保留 [LITE] 标记的部分
# 同时接受上游的其他修改
```

### Step 4: 处理正常 Merge 的文件

```bash
# 接受上游版本
git checkout --theirs <file>

# 或者保留默认合并结果
git add <file>
```

### Step 5: 更新 VERSION 文件

```bash
# 读取上游版本号
UPSTREAM_VERSION=$(cat backend/cmd/server/VERSION | tr -d '[:space:]')

# 自动追加 -Lite 后缀
echo "${UPSTREAM_VERSION}-Lite" > backend/cmd/server/VERSION

# 验证
cat backend/cmd/server/VERSION
# 期望输出: 0.1.88-Lite (举例)
```

> **注意**: 上游 VERSION 格式为 `0.1.88`，Lite 版本格式为 `0.1.88-Lite`

### Step 6: 重新生成

```bash
cd backend

# 重新生成 Ent
go generate ./ent

# 重新生成 Wire
cd cmd/server && wire && cd ../..
```

### Step 7: 编译验证

```bash
cd backend

# 编译
go build ./...

# 测试
go test ./...
```

### Step 8: 提交

```bash
git add .
git commit -m "chore: merge upstream vX.X.X, preserve lite changes"
```

---

## Part 5: 验证清单

完成 merge 后，逐项检查：

```
编译和测试
[ ] go build ./... 通过
[ ] go test ./... 通过

版本验证
[ ] backend/cmd/server/VERSION 包含 X.X.X-Lite 格式

删除文件验证
[ ] auth_handler.go 不存在
[ ] redeem_*.go 不存在
[ ] promo_*.go 不存在
[ ] user_attribute*.go 不存在
[ ] routes/auth.go, routes/user.go 不存在

Lite 代码验证
[ ] config.go 包含 RunModeLite 常量
[ ] billing_cache_service.go 包含 checkUsageLimits 函数
[ ] gateway_service.go 包含 recordUsageLite 函数
[ ] api_key.go schema 包含限额字段
[ ] routes/admin.go 包含 [LITE:SKIP] 条件

运行时验证
[ ] RUN_MODE=lite 配置生效
[ ] /v1/usage 返回限额格式
[ ] API Key 限额检查正常工作
```

---

## Part 6: 基础信息

```yaml
项目: Sub2API Lite
上游仓库: https://github.com/Wei-Shaw/sub2api
基于版本: v0.1.109
分支: lite/main
迁移文件起始: 100
```

---

## Merge 历史

| 日期 | 上游版本 | 处理结果 | 备注 |
|------|----------|----------|------|
| 2026-01-10 | v0.1.35 | 初始版本 | 创建 Lite 分支 |
| 2026-02-17 | v0.1.76 | 成功 | 大规模 merge (626 commits, 92 冲突) |
| 2026-02-19 | v0.1.83 | 成功 | 轻量 merge (7 commits, 2 冲突) |
| 2026-03-15 | v0.1.88 | 修复 | 大规模重构 (856 files, +188k/-49k)，前端未受影响 |
| 2026-03-28 | v0.1.103 | 成功 | 同步 DB备份/Backend Mode/端点可观测性/upstream model/分组容量等 (116 commits) |
| 2026-04-08 | v0.1.109 | 成功 | 同步 Channel定价/Sora移除/gpt-5.4-mini/虚拟滚动/requested model等 (236 commits) |

### v0.1.88 Merge 问题记录

**发现日期**: 2026-03-16

**问题 1: `CheckBillingEligibility` 被上游余额检查覆盖**
- **现象**: 非订阅模式下请求被 `ErrInsufficientBalance` 拦截，Lite 限额检查根本走不到
- **原因**: 上游新增 `checkBalanceEligibility` 和 `checkAPIKeyRateLimits`，把 Lite 的 `return` 改成 `if err`
- **修复**: 恢复 v0.1.83 逻辑 — 非订阅模式直接 `return checkAPIKeyLimitEligibility`，跳过余额检查
- **文件**: `backend/internal/service/billing_cache_service.go`

**问题 2: `listSchedulableAccounts` mixed 分支漏加 Lite**
- **现象**: Lite 模式在 Anthropic/Gemini 平台只能获取无分组账号
- **原因**: 上游新增 `RunModeSimple` 条件分支，无 groupID 时 Lite 走到 `ListSchedulableUngroupedByPlatforms`
- **修复**: 恢复 v0.1.83 逻辑 — 无 groupID 时直接 `ListSchedulableByPlatforms` 查所有
- **文件**: `backend/internal/service/gateway_service.go`

**前端增强**:
- 分组计费类型显示改为 "Lite 模式（无余额限制）"，standard 分组也可设限额字段
- 设置页 Security tab 精简：删除注册/邀请码/优惠码/Turnstile/LinuxDO，保留邮箱验证+密码重置+TOTP

**问题 3: TOTP 2FA 路由未注册**
- **现象**: 前端点击 2FA 设置直接报错（404）
- **原因**: Lite 的 `routes/user.go` 是简化版，漏掉了 `/user/totp/*` 路由组
- **修复**: 在 user 路由组下添加完整 TOTP 路由（status/verification-method/send-code/setup/enable/disable）
- **文件**: `backend/internal/server/routes/user.go`

### v0.1.109 Merge 记录

**日期**: 2026-04-08

**特点**: 上游在 v0.1.107 完全移除了 Sora 平台，大量 Sora 相关文件被清理。Channel 定价系统全面上线。236 non-merge commits。

**Lite 适配工作**:
1. 删除 5 个被 merge 恢复的 ent schema（promo_code/redeem_code/user_attribute_*）
2. 删除 `identity_service_order_test.go`（上游新增测试，依赖已删除的 NewIdentityService）
3. 新增 `ProvideOAuthRefreshAPI` wrapper — wire v0.7.0 无法处理 variadic 参数 `lockTTL ...time.Duration`
4. 修复 `cmd/server/wire.go` 语法错误（OpsSystemLogSink 后残留闭合括号）
5. 前端修复：安装 `@tanstack/vue-virtual`、移除空路由和不存在的 DataManagementView 引用

**注意**: 上游 Sora 移除后，`user_allowed_group.go` 不再保留决策中的 "被 sora_quota_service 使用" 理由。但 admin_service/api_key_service 仍然引用 AllowedGroups，保留决策不变。

### v0.1.109 Bugfix: OpenAI 账号调度 groupID 修复

**发现日期**: 2026-04-09

**问题**: `/v1/responses` 请求 OpenAI 模型（如 gpt-5.4）时返回 `503 Service temporarily unavailable`，日志报 `no available OpenAI accounts`，即使 OpenAI 账号已正确分配到分组。

**根本原因**: Lite 的 `openai_gateway_service.go:listSchedulableAccounts` 传 `nil` 作为 `groupID` 给 `schedulerSnapshot.ListSchedulableAccounts()`。snapshot 的 `normalizeGroupID(nil)` 返回 0，`loadAccountsFromDB` 在 `groupID=0` + 非 simple 模式下走到 `ListSchedulableUngroupedByPlatform`——**只返回没分配任何分组的账号**。已分配到分组的 OpenAI 账号全部被排除。

**修复**:
- `openai_gateway_service.go` — 传真实 `groupID` 给 snapshot 和 repo 查询
- `gateway_service.go` — fallback 路径也改为尊重 groupID

**教训**: Lite 修改分组逻辑时，不能简单传 `nil` 来"忽略分组限制"。在 snapshot 层，`nil/0` groupID 实际含义是"查无分组账号"，而非"查所有账号"。

