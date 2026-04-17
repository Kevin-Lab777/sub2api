# Sub2API Lite 变更清单

> 此文档记录 Lite 版本相对于上游 Standard 版本的所有变更
> 用于指导上游更新的 merge 过程

## 版本信息

| 项目 | 值 |
|------|-----|
| 基于上游版本 | v0.1.103 |
| 最后同步日期 | 2026-03-28 |
| Lite 版本 | v0.1.103-lite |
| 上游仓库 | https://github.com/Wei-Shaw/sub2api |

---

## 快速参考

### 标记说明

| 标记 | 含义 | Merge 策略 |
|------|------|------------|
| `DELETED` | 文件已删除 | 上游更新时忽略 |
| `MODIFIED` | 有 Lite 修改 | 手动合并，保留 Lite 逻辑 |
| `ADDED` | Lite 新增 | 保留新增 |
| `NORMAL` | 无修改 | 正常接受上游更新 |

### Merge 优先级

```
🔴 忽略上游 - 我们已删除的文件，上游改了也不管
🟡 手动审查 - 有 Lite 修改的文件，逐行对比合并
🟢 正常 Merge - 接受上游变更
```

---

## 1. 已删除的文件 🔴

> 这些文件已从 Lite 版本删除，上游更新时直接忽略

### 1.1 用户认证系统

| 文件 | 状态 |
|------|------|
| `backend/internal/handler/auth_handler.go` | DELETED |
| `backend/internal/handler/auth_linuxdo_oauth.go` | DELETED |
| `backend/internal/handler/user_handler.go` | DELETED |
| `backend/internal/service/auth_service.go` | DELETED |
| `backend/internal/service/identity_service.go` | DELETED |
| `backend/internal/service/turnstile_service.go` | DELETED |
| `backend/internal/service/email_service.go` | DELETED |
| `backend/internal/service/email_queue_service.go` | DELETED |

### 1.2 兑换码系统

| 文件 | 状态 |
|------|------|
| `backend/internal/handler/redeem_handler.go` | DELETED |
| `backend/internal/handler/admin/redeem_handler.go` | DELETED |
| `backend/internal/service/redeem_service.go` | DELETED |
| `backend/internal/service/redeem_code.go` | DELETED |
| `backend/ent/schema/redeem_code.go` | DELETED |

### 1.3 优惠码系统

| 文件 | 状态 |
|------|------|
| `backend/internal/handler/admin/promo_handler.go` | DELETED |
| `backend/internal/service/promo_service.go` | DELETED |
| `backend/internal/service/promo_code.go` | DELETED |
| `backend/internal/service/promo_code_repository.go` | DELETED |
| `backend/ent/schema/promo_code.go` | DELETED |
| `backend/ent/schema/promo_code_usage.go` | DELETED |

### 1.4 用户属性系统

| 文件 | 状态 |
|------|------|
| `backend/internal/handler/admin/user_attribute_handler.go` | DELETED |
| `backend/internal/service/user_attribute.go` | DELETED |
| `backend/internal/service/user_attribute_service.go` | DELETED |
| `backend/ent/schema/user_attribute_definition.go` | DELETED |
| `backend/ent/schema/user_attribute_value.go` | DELETED |
| `backend/ent/schema/user_allowed_group.go` | KEPT (v0.1.88) | 上游升级为核心功能，保留 + 添加 id 列迁移 |

### 1.5 用户端 API

| 文件 | 状态 | 说明 |
|------|------|------|
| `backend/internal/handler/api_key_handler.go` | DELETED | 由 Admin 统一管理 |
| `backend/internal/handler/usage_handler.go` | DELETED | 由 Admin 统一管理 |
| `backend/internal/handler/subscription_handler.go` | DELETED | 由 Admin 统一管理 |

---

## 2. 保留但修改的文件 🟡

> 这些文件有 Lite 专属修改，merge 时需要手动审查

### 2.1 config/config.go

```
状态: MODIFIED
关键修改: 标准模式行为改变
```

**Lite 逻辑**:
- 标准模式不扣余额
- 支持可选的 Group/API Key 限额控制

### 2.2 billing_cache_service.go

```
状态: MODIFIED
关键函数: CheckBillingEligibility
```

**Lite 逻辑**:
```go
// 标准模式：检查可选限额
if billingType == BillingTypeStandard {
    return s.checkOptionalLimits(ctx, apiKey, group)
}
```

### 2.3 gateway_service.go

```
状态: MODIFIED
关键函数: RecordUsage
```

**Lite 逻辑**:
```go
// 标准模式：只记录用量，不扣余额
if billingType == BillingTypeStandard {
    return s.recordUsageOnly(ctx, input)
}
```

### 2.4 gateway_handler.go

```
状态: MODIFIED
关键函数: Usage
```

**Lite 逻辑**: 返回限额信息而不是余额

### 2.5 routes/auth.go

```
状态: MODIFIED
```

**Lite 逻辑**: 只注册 login/settings/public/auth/me，不注册 register/verify/oauth

### 2.6 routes/user.go

```
状态: MODIFIED
```

**Lite 逻辑**: 保留 /user/profile, /user/password, /keys/*, /usage/*, /groups/available, /user/totp/*
删除 /redeem/*, /subscriptions/*

### 2.7 routes/admin.go

```
状态: MODIFIED
```

**Lite 逻辑**: 移除兑换码/优惠码/用户属性路由注册

### 2.7 ent/schema/api_key.go

```
状态: MODIFIED
```

**新增字段**:
```go
// 限额字段
field.Float("daily_limit_usd").Optional().Nillable()
field.Float("weekly_limit_usd").Optional().Nillable()
field.Float("monthly_limit_usd").Optional().Nillable()
field.Float("total_limit_usd").Optional().Nillable()

// 用量追踪字段
field.Float("daily_usage_usd").Default(0)
field.Float("weekly_usage_usd").Default(0)
field.Float("monthly_usage_usd").Default(0)
field.Float("total_usage_usd").Default(0)
field.Time("usage_reset_daily").Optional().Nillable()
field.Time("usage_reset_weekly").Optional().Nillable()
field.Time("usage_reset_monthly").Optional().Nillable()
```

### 2.8 Wire 依赖注入

```
状态: MODIFIED
文件: wire.go, wire_gen.go (多处)
```

**Lite 逻辑**: 移除已删除服务的依赖注入

---

## 3. Lite 新增文件 🟢

| 文件 | 说明 |
|------|------|
| `backend/internal/service/api_key_usage_service.go` | API Key 用量追踪 |
| `backend/migrations/100_lite_mode.sql` | 数据库迁移 |
| `backend/migrations/101_add_user_allowed_groups_id.sql` | user_allowed_groups 添加 id 列 |
| `backend/internal/repository/api_key_usage_repo.go` | API Key 用量 Repository |
| `frontend/src/api/auth.ts` | Lite 版 Auth API (login/logout/getCurrentUser) |

---

## 4. 正常 Merge 文件 🟢

以下文件/目录可以直接接受上游更新：

```
backend/internal/handler/admin/account_handler.go
backend/internal/handler/admin/proxy_handler.go
backend/internal/handler/admin/group_handler.go
backend/internal/handler/admin/dashboard_handler.go
backend/internal/handler/admin/setting_handler.go
backend/internal/handler/admin/system_handler.go
backend/internal/handler/admin/subscription_handler.go  # 订阅保留
backend/internal/handler/admin/usage_handler.go
backend/internal/handler/admin/*_oauth_handler.go

backend/internal/service/account*.go
backend/internal/service/proxy*.go
backend/internal/service/group*.go
backend/internal/service/dashboard_service.go
backend/internal/service/setting*.go
backend/internal/service/subscription_service.go  # 订阅保留
backend/internal/service/usage*.go
backend/internal/service/*_oauth_service.go

backend/internal/middleware/
backend/internal/pkg/
backend/migrations/  # 我们的从 100 开始，不冲突

frontend/
```

---

## 5. 上游同步检查清单

当上游发布新版本时：

### 5.1 直接忽略 (我们已删除)

- [ ] `auth_*.go`
- [ ] `user_handler.go`
- [ ] `redeem_*.go`
- [ ] `promo_*.go`
- [ ] `user_attribute*.go`
- [ ] `identity_service.go`
- [ ] `turnstile_service.go`
- [ ] `email_*.go`

### 5.2 手动审查 (有 Lite 修改)

- [ ] `billing_cache_service.go` - 保留限额检查逻辑
- [ ] `gateway_service.go` - 保留标准模式不扣余额
- [ ] `gateway_handler.go` - 保留 Usage 返回限额
- [ ] `api_key.go` (schema) - 保留新增字段
- [ ] `routes/auth.go` - 保留只注册公开设置
- [ ] `routes/admin.go` - 保留移除的路由
- [ ] `wire*.go` - 保留依赖注入修改

### 5.3 正常 Merge

- [ ] 账号/代理/分组管理
- [ ] 仪表盘
- [ ] 系统设置
- [ ] 订阅管理 (Admin)
- [ ] OAuth 相关
- [ ] 前端

---

## 6. 版本历史

| 版本 | 日期 | 基于上游 | 说明 |
|------|------|----------|------|
| v0.1.35-lite.1 | 2026-01-10 | v0.1.35 | 初始 Lite 版本 |
| v0.1.46-lite | 2026-01-11 | v0.1.46 | 同步上游更新 |
| v0.1.50-lite | 2026-01-12 | v0.1.50 | 同步 Ops 监控系统 |
| v0.1.53-lite | 2026-01-13 | v0.1.53 | 恢复用户 Profile 页面 |
| v0.1.76-lite | 2026-02-17 | v0.1.76 | 同步 TOTP/公告/错误透传等功能 |
| v0.1.83-lite | 2026-02-19 | v0.1.83 | 同步 Cache TTL Override/Codex 修复等 |
| v0.1.88-lite | 2026-03-16 | v0.1.88 | 同步 Sora/IP白名单/quota+rate_limit/幂等性等 + 修复计费/账号调度/TOTP/前端精简 |
| v0.1.103-lite | 2026-03-28 | v0.1.103 | 同步 DB备份/Backend Mode/端点可观测性/upstream model追踪/分组容量列/529 cooldown等 |
