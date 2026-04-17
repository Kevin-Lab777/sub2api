# Sub2API Lite 技术规格文档

> 基于 Wei-Shaw/sub2api v0.1.109
> 版本: Lite (轻量化个人/小团队版本)
> 日期: 2026-01-10
> 最后更新: 2026-04-08

---

## 1. 项目概述

### 1.1 目标

将 Sub2API 从多用户 SaaS 模式精简为面向**个人/小团队**的轻量化版本：

- 移除多用户注册/登录系统（只保留 Admin）
- 移除兑换码/优惠码系统
- 保留核心 API 网关能力
- **保留订阅系统**
- 标准模式改为**限额控制**（不扣余额）

### 1.2 版本对比

| 功能 | 原版 Standard | Lite 版本 |
|------|--------------|-----------|
| 用户注册/登录 | ✅ | ❌ 移除 |
| 多用户管理 | ✅ | ❌ 移除 (仅 Admin) |
| 兑换码系统 | ✅ | ❌ 移除 |
| 优惠码系统 | ✅ | ❌ 移除 |
| 用户属性 | ✅ | ❌ 移除 |
| simple/standard 模式切换 | ✅ | ❌ 移除 (只有 lite) |
| **订阅系统** | ✅ | ✅ 保留 |
| **标准模式计费** | 扣余额 | 纯限额控制 |
| 仪表盘 | ✅ | ✅ 保留 |
| 分组管理 | ✅ | ✅ 保留 |
| 账号管理 | ✅ | ✅ 保留 |
| 代理管理 | ✅ | ✅ 保留 |
| API Key 管理 | ✅ | ✅ 增强 (支持限额) |
| 使用记录 | ✅ | ✅ 保留 |
| 系统设置 | ✅ | ✅ 保留 |

---

## 2. 计费模型

### 2.1 两种模式

Lite 版本保留原版的两种计费模式，但**标准模式行为改变**：

| 模式 | 原版行为 | Lite 行为 |
|------|----------|-----------|
| **标准** | 扣用户余额 | **无限制（不扣余额）**，可选限额控制 |
| **订阅（配额）** | 订阅配额限制 | 保持不变 |

### 2.2 标准模式 - 默认无限制 + 可选限额

```
┌──────────────────────────────────────────────────────────┐
│                    标准模式 (Lite)                        │
├──────────────────────────────────────────────────────────┤
│  • 默认无限制 - 不扣余额，随便用                          │
│  • 可选限额控制：                                         │
│    - Group 级别：日/周/月限额                             │
│    - API Key 级别：日/周/月限额 或 总额限制               │
│  • 记录用量 - 用于统计和可选的限额检查                    │
└──────────────────────────────────────────────────────────┘
```

### 2.3 限额优先级

```
API Key 有设置限额？
    │
    ├─ YES → 使用 API Key 限额
    │        (daily/weekly/monthly/total_limit_usd)
    │
    └─ NO
        │
        Group 有设置限额？
            │
            ├─ YES → 使用 Group 限额
            │        (daily/weekly/monthly_limit_usd)
            │
            └─ NO → 无限制（随便用）
```

### 2.4 订阅模式

保持原版逻辑不变：
- 用户订阅分组
- 按订阅配额限制使用
- 支持日/周/月配额

---

## 3. 需要删除的代码

### 3.1 删除的文件列表

**用户认证相关：**
```
backend/internal/handler/auth_handler.go
backend/internal/handler/auth_linuxdo_oauth.go
backend/internal/handler/user_handler.go
backend/internal/service/auth_service.go
backend/internal/service/identity_service.go
backend/internal/service/turnstile_service.go
backend/internal/service/email_service.go
backend/internal/service/email_queue_service.go
```

**兑换码系统：**
```
backend/internal/handler/redeem_handler.go
backend/internal/handler/admin/redeem_handler.go
backend/internal/service/redeem_service.go
backend/internal/service/redeem_code.go
backend/ent/schema/redeem_code.go
```

**优惠码系统：**
```
backend/internal/handler/admin/promo_handler.go
backend/internal/service/promo_service.go
backend/internal/service/promo_code.go
backend/internal/service/promo_code_repository.go
backend/ent/schema/promo_code.go
backend/ent/schema/promo_code_usage.go
```

**用户属性系统：**
```
backend/internal/handler/admin/user_attribute_handler.go
backend/internal/service/user_attribute.go
backend/internal/service/user_attribute_service.go
backend/ent/schema/user_attribute_definition.go
backend/ent/schema/user_attribute_value.go
backend/ent/schema/user_allowed_group.go
```

**用户端 API（由 Admin 统一管理）：**
```
backend/internal/handler/api_key_handler.go
backend/internal/handler/usage_handler.go
backend/internal/handler/subscription_handler.go
```

### 3.2 保留的文件

**订阅系统（保留）：**
```
backend/internal/handler/admin/subscription_handler.go  ✓ 保留
backend/internal/service/subscription_service.go        ✓ 保留
backend/internal/service/user_subscription.go           ✓ 保留
backend/ent/schema/user_subscription.go                 ✓ 保留
```

---

## 4. Schema 变更

### 4.1 API Key Schema 增强

**文件**: `backend/ent/schema/api_key.go`

```go
// 新增限额字段
field.Float("daily_limit_usd").
    Optional().
    Nillable().
    SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
    Comment("日限额 (USD)，nil = 使用分组限额"),

field.Float("weekly_limit_usd").
    Optional().
    Nillable().
    SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
    Comment("周限额 (USD)，nil = 使用分组限额"),

field.Float("monthly_limit_usd").
    Optional().
    Nillable().
    SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
    Comment("月限额 (USD)，nil = 使用分组限额"),

field.Float("total_limit_usd").
    Optional().
    Nillable().
    SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
    Comment("总限额 (USD)，nil = 无限制"),

// 用量追踪字段
field.Float("daily_usage_usd").
    Default(0).
    SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
    Comment("今日已用额度"),

field.Float("weekly_usage_usd").
    Default(0).
    SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
    Comment("本周已用额度"),

field.Float("monthly_usage_usd").
    Default(0).
    SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
    Comment("本月已用额度"),

field.Float("total_usage_usd").
    Default(0).
    SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
    Comment("累计已用额度"),

field.Time("usage_reset_daily").
    Optional().
    Nillable().
    Comment("日用量重置时间"),

field.Time("usage_reset_weekly").
    Optional().
    Nillable().
    Comment("周用量重置时间"),

field.Time("usage_reset_monthly").
    Optional().
    Nillable().
    Comment("月用量重置时间"),
```

### 4.2 User Schema 简化

**文件**: `backend/ent/schema/user.go`

移除的关联（Edge）：
- `redeem_codes` - 删除
- `promo_code_usages` - 删除
- `attribute_values` - 删除

保留的关联：
- `subscriptions` - 保留（订阅系统需要）
- `api_keys` - 保留

---

## 5. API 变更

### 5.1 删除的路由

**认证模块** (`/api/v1/auth`):
```diff
- POST /register
- POST /login
- POST /send-verify-code
- POST /validate-promo-code
- GET  /oauth/linuxdo/*
- GET  /me
```

**用户模块** (`/api/v1/user`):
```diff
- 整个模块删除
```

**用户端 API Key** (`/api/v1/keys`):
```diff
- 整个模块删除 (由管理后台统一管理)
```

**用户端兑换** (`/api/v1/redeem`):
```diff
- 整个模块删除
```

**用户端订阅** (`/api/v1/subscriptions`):
```diff
- 整个模块删除 (由管理后台统一管理)
```

**Admin 路由删除部分**:
```diff
- /api/v1/admin/users/:id/attributes
- /api/v1/admin/user-attributes/*
- /api/v1/admin/redeem-codes/*
- /api/v1/admin/promo-codes/*
```

### 5.2 保留的路由

**Admin 订阅管理** (保留):
```
GET    /api/v1/admin/subscriptions
GET    /api/v1/admin/subscriptions/:id
POST   /api/v1/admin/subscriptions/assign
POST   /api/v1/admin/subscriptions/bulk-assign
POST   /api/v1/admin/subscriptions/:id/extend
DELETE /api/v1/admin/subscriptions/:id
```

**Admin API Key 管理** (增强):
```
GET    /api/v1/admin/api-keys
GET    /api/v1/admin/api-keys/:id
POST   /api/v1/admin/api-keys
PUT    /api/v1/admin/api-keys/:id
DELETE /api/v1/admin/api-keys/:id
GET    /api/v1/admin/api-keys/:id/usage
POST   /api/v1/admin/api-keys/:id/reset-usage  # 新增
```

**Gateway API** (保留，标准模式逻辑调整):
```
POST /v1/messages           # 标准模式改为限额检查
GET  /v1/usage              # 标准模式返回限额信息
... (其他保留)
```

---

## 6. Service 层变更

### 6.1 删除的 Service

```
backend/internal/service/auth_service.go
backend/internal/service/identity_service.go
backend/internal/service/turnstile_service.go
backend/internal/service/email_service.go
backend/internal/service/email_queue_service.go
backend/internal/service/redeem_service.go
backend/internal/service/redeem_code.go
backend/internal/service/promo_service.go
backend/internal/service/promo_code.go
backend/internal/service/promo_code_repository.go
backend/internal/service/user_attribute.go
backend/internal/service/user_attribute_service.go
```

### 6.2 保留的 Service

```
backend/internal/service/subscription_service.go    ✓ 保留
backend/internal/service/user_subscription.go       ✓ 保留
```

### 6.3 修改的 Service

**billing_cache_service.go**:
```go
func (s *BillingCacheService) CheckBillingEligibility(...) error {
    // Lite 标准模式：限额检查（不扣余额）
    if billingType == BillingTypeStandard {
        return s.checkUsageLimits(ctx, apiKey, group)
    }

    // 订阅模式：保持原逻辑
    if billingType == BillingTypeSubscription {
        return s.checkSubscriptionEligibility(ctx, apiKey, group)
    }

    return nil
}

// 新增: 限额检查函数
func (s *BillingCacheService) checkUsageLimits(ctx context.Context, apiKey *APIKey, group *Group) error {
    // API Key 限额优先于 Group 限额
    // ...
}
```

**gateway_service.go**:
```go
func (s *GatewayService) RecordUsage(ctx context.Context, input *RecordUsageInput) error {
    // 1. 记录使用日志 (保留)

    // 2. 标准模式：更新 API Key 用量统计，不扣余额
    if billingType == BillingTypeStandard {
        return s.updateAPIKeyUsage(ctx, input)
    }

    // 3. 订阅模式：保持原逻辑
    if billingType == BillingTypeSubscription {
        return s.updateSubscriptionUsage(ctx, input)
    }
}
```

### 6.4 新增的 Service

**api_key_usage_service.go**:
```go
type APIKeyUsageService struct {
    db    *ent.Client
    cache *redis.Client
}

func (s *APIKeyUsageService) GetUsage(ctx context.Context, apiKeyID int64) (*APIKeyUsage, error)
func (s *APIKeyUsageService) IncrementUsage(ctx context.Context, apiKeyID int64, costUSD float64) error
func (s *APIKeyUsageService) ResetUsage(ctx context.Context, apiKeyID int64, period string) error
func (s *APIKeyUsageService) CheckAndResetExpiredPeriods(ctx context.Context, apiKeyID int64) error
```

---

## 7. Handler 层变更

### 7.1 删除的 Handler

```
backend/internal/handler/auth_handler.go
backend/internal/handler/auth_linuxdo_oauth.go
backend/internal/handler/user_handler.go
backend/internal/handler/api_key_handler.go (用户端)
backend/internal/handler/usage_handler.go (用户端)
backend/internal/handler/subscription_handler.go (用户端)
backend/internal/handler/redeem_handler.go
backend/internal/handler/admin/redeem_handler.go
backend/internal/handler/admin/promo_handler.go
backend/internal/handler/admin/user_attribute_handler.go
```

### 7.2 保留的 Handler

```
backend/internal/handler/admin/subscription_handler.go  ✓ 保留
```

### 7.3 修改的 Handler

**gateway_handler.go**:
```go
func (h *GatewayHandler) Usage(c *gin.Context) {
    // 标准模式：返回限额信息
    if billingType == BillingTypeStandard {
        h.usageWithLimits(c)
        return
    }

    // 订阅模式：保持原逻辑
    // ...
}
```

---

## 8. 路由注册变更

### 8.1 routes/auth.go

```go
func RegisterAuthRoutes(r *gin.RouterGroup, h *handler.Handler, cfg *config.Config) {
    // Lite 版本：只注册公开设置
    settings := r.Group("/settings")
    settings.GET("/public", h.Setting.GetPublicSettings)

    // 不注册用户认证路由
}
```

### 8.2 routes/admin.go

```go
func RegisterAdminRoutes(...) {
    // 保留的路由
    registerAccountRoutes(admin, h)
    registerProxyRoutes(admin, h)
    registerGroupRoutes(admin, h)
    registerAPIKeyRoutes(admin, h)  // 增强版
    registerUsageRoutes(admin, h)
    registerDashboardRoutes(admin, h)
    registerSettingRoutes(admin, h)
    registerSystemRoutes(admin, h)
    registerSubscriptionRoutes(admin, h)  // 保留

    // 删除的路由
    // registerRedeemCodeRoutes - 删除
    // registerPromoCodeRoutes - 删除
    // registerUserAttributeRoutes - 删除
}
```

---

## 9. 数据库迁移

**`migrations/100_lite_mode.sql`**:
```sql
-- 1. API Key 新增限额和用量字段
ALTER TABLE api_keys
ADD COLUMN IF NOT EXISTS daily_limit_usd DECIMAL(20,8) NULL,
ADD COLUMN IF NOT EXISTS weekly_limit_usd DECIMAL(20,8) NULL,
ADD COLUMN IF NOT EXISTS monthly_limit_usd DECIMAL(20,8) NULL,
ADD COLUMN IF NOT EXISTS total_limit_usd DECIMAL(20,8) NULL,
ADD COLUMN IF NOT EXISTS daily_usage_usd DECIMAL(20,8) NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS weekly_usage_usd DECIMAL(20,8) NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS monthly_usage_usd DECIMAL(20,8) NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS total_usage_usd DECIMAL(20,8) NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS usage_reset_daily TIMESTAMPTZ NULL,
ADD COLUMN IF NOT EXISTS usage_reset_weekly TIMESTAMPTZ NULL,
ADD COLUMN IF NOT EXISTS usage_reset_monthly TIMESTAMPTZ NULL;

-- 2. 创建用量追踪索引
CREATE INDEX IF NOT EXISTS idx_api_keys_usage_reset
ON api_keys(usage_reset_daily, usage_reset_weekly, usage_reset_monthly);

-- 3. 可选：清理不需要的表（建议保留数据以备回滚）
-- DROP TABLE IF EXISTS redeem_codes CASCADE;
-- DROP TABLE IF EXISTS promo_codes CASCADE;
-- DROP TABLE IF EXISTS promo_code_usages CASCADE;
-- DROP TABLE IF EXISTS user_attribute_definitions CASCADE;
-- DROP TABLE IF EXISTS user_attribute_values CASCADE;
-- DROP TABLE IF EXISTS user_allowed_groups CASCADE;
```

---

## 10. 配置变更

### 10.1 移除的配置项

```yaml
# config.yaml 移除项
run_mode: standard    # 移除，固定为 lite

linuxdo_connect:      # 整个移除
  enabled: false

turnstile:            # 移除验证码
  required: false
```

### 10.2 保留的配置项

```yaml
# 保留订阅相关配置
default:
  admin_email: ""
  admin_password: ""
```

### 10.3 新增的配置项

```yaml
# Lite 模式专用配置
lite:
  # 默认 API Key 限额 (创建时使用)
  default_daily_limit_usd: null
  default_weekly_limit_usd: null
  default_monthly_limit_usd: null
  default_total_limit_usd: null

  # 用量重置时区
  usage_reset_timezone: "Asia/Shanghai"
```

---

## 11. 实现步骤

### Phase 1: 删除不需要的代码 ✅ 完成

1. [x] 删除用户认证相关文件
2. [x] 删除兑换码系统文件
3. [x] 删除优惠码系统文件
4. [x] 删除用户属性系统文件
5. [x] 删除用户端 Handler 文件
6. [x] 更新路由注册，移除相关路由
7. [x] 更新 Wire 依赖注入
8. [x] 编译测试

### Phase 2: Schema 和数据库 ✅ 完成

1. [x] 修改 `api_key.go` schema，添加限额和用量字段
2. [x] 修改 `user.go` schema，移除不需要的 Edge
3. [x] 创建迁移文件
4. [x] 运行 `go generate ./ent`
5. [x] 测试数据库迁移

### Phase 3: 标准模式限额逻辑 ✅ 完成

1. [x] 创建 `api_key_usage_service.go`
2. [x] 修改 `billing_cache_service.go`，添加限额检查 (`checkAPIKeyLimitEligibility`)
3. [x] 修改 `gateway_service.go`，标准模式不扣余额 (LITE 分支用量记录)
4. [x] 修改 `gateway_handler.go`，Usage 返回限额信息 (`calculateAPIKeyRemaining`)
5. [x] 添加用量重置定时任务 (按需重置: `CheckAndResetExpiredPeriods`)
6. [x] Wire 依赖注入配置 (`ProvideAPIKeyUsageRepository`)

### Phase 4: Admin API 增强 ✅ 完成

1. [x] 增强 API Key 管理 Handler，支持限额字段
2. [x] 添加重置用量 API (`POST /api/v1/admin/api-keys/:id/reset-usage`)
3. [x] 更新 DTO (`dto/types.go`, `dto/mappers.go`)

### Phase 5: 前端适配

#### 5.1 删除的文件

**认证页面 (整个 auth 目录删除)**:
```
frontend/src/views/auth/LoginView.vue
frontend/src/views/auth/RegisterView.vue
frontend/src/views/auth/EmailVerifyView.vue
frontend/src/views/auth/LinuxDoCallbackView.vue
frontend/src/views/auth/OAuthCallbackView.vue
frontend/src/views/auth/index.ts
frontend/src/views/auth/*.md (文档文件)
```

**用户端页面 (整个 user 目录删除)**:
```
frontend/src/views/user/DashboardView.vue
frontend/src/views/user/KeysView.vue
frontend/src/views/user/ProfileView.vue
frontend/src/views/user/RedeemView.vue
frontend/src/views/user/SubscriptionsView.vue
frontend/src/views/user/UsageView.vue
```

**Admin 删除页面**:
```
frontend/src/views/admin/RedeemView.vue       # 兑换码管理
frontend/src/views/admin/PromoCodesView.vue   # 优惠码管理
frontend/src/views/admin/UsersView.vue        # 用户管理
```

**API 文件删除**:
```
frontend/src/api/auth.ts      # 认证 API
frontend/src/api/user.ts      # 用户 API
frontend/src/api/redeem.ts    # 兑换码 API
```

#### 5.2 修改的文件

**路由文件** `frontend/src/router/index.ts`:
- 移除 `/login`, `/register`, `/verify-email` 等认证路由
- 移除 `/user/*` 用户端路由
- 移除 `/admin/redeem`, `/admin/promo`, `/admin/users` 路由
- 设置 `/admin` 为默认首页

**导航/菜单组件**:
- 移除用户端导航入口
- 移除 Admin 菜单中的：兑换码管理、优惠码管理、用户管理

**Settings 页面** `frontend/src/views/admin/SettingsView.vue`:
- 移除邮件配置 (SMTP) 相关 UI
- 移除 Turnstile 验证码配置 UI
- 移除 LinuxDO Connect 配置 UI

#### 5.3 新增/增强的 UI

**API Key 管理增强** `frontend/src/views/admin/GroupsView.vue` (API Key 在 Groups 里管理):
- 添加限额字段输入：日限额、周限额、月限额
- 添加当前用量显示：日用量、周用量、月用量
- 添加重置用量按钮
- 添加限额进度条可视化

**API 文件增强** `frontend/src/api/admin/`:
- 新增 `resetAPIKeyUsage(id)` 方法
- 更新 API Key DTO 包含限额字段

#### 5.4 前端实现步骤

1. [ ] 删除 `views/auth/` 目录
2. [ ] 删除 `views/user/` 目录
3. [ ] 删除 `views/admin/RedeemView.vue`
4. [ ] 删除 `views/admin/PromoCodesView.vue`
5. [ ] 删除 `views/admin/UsersView.vue`
6. [ ] 删除 `api/auth.ts`, `api/user.ts`, `api/redeem.ts`
7. [ ] 修改 `router/index.ts` 移除相关路由
8. [ ] 修改导航组件移除菜单项
9. [ ] 修改 `SettingsView.vue` 移除邮件/Turnstile 配置
10. [ ] 增强 API Key 管理 UI (限额字段)
11. [ ] 编译测试前端

### Phase 6: 测试和文档

1. [ ] 单元测试
2. [ ] 集成测试
3. [ ] 更新 README
4. [ ] 更新部署文档

---

## 12. 总结

Lite 版本核心改动：

1. **移除多用户系统** - 只保留管理员
2. **移除兑换码/优惠码** - 简化系统
3. **保留订阅系统** - 支持订阅配额模式
4. **标准模式改为纯限额** - 不扣余额，API Key/分组限额控制
5. **API Key 增强** - 支持独立限额和用量追踪

预计工作量：**8-10 天**
