# Sub2API Lite 变更清单

> 此文档记录 Lite 版本相对于上游 Standard 版本的所有变更
> 用于指导上游更新的 merge 过程

## 版本信息

| 项目 | 值 |
|------|-----|
| 基于上游版本 | v0.1.109 |
| 最后同步日期 | 2026-04-08 |
| Lite 版本 | v0.1.109-lite |
| 上游仓库 | https://github.com/Wei-Shaw/sub2api |
| Fork 仓库 | https://github.com/Kevin-Lab777/sub2api |
| Light 分支 | https://github.com/Kevin-Lab777/sub2api/tree/Light |

---

## 快速参考

### 标记说明

| 标记 | 含义 | Merge 策略 |
|------|------|------------|
| `DELETED` | 文件已完全删除 | 上游更新时 `git rm` |
| `SIMPLIFIED` | 文件简化保留 | 保留 Lite 版本，拒绝上游覆盖 |
| `MODIFIED` | 有 Lite 修改 | 手动合并，保留 Lite 逻辑 |
| `ADDED` | Lite 新增 | 保留新增 |
| `NORMAL` | 无修改 | 正常接受上游更新 |

### Merge 优先级

```
🔴 忽略上游 - 我们已删除的文件，上游改了也不管
🟡 保留简化 - 有 [LITE] 标记的简化版本，拒绝上游覆盖
🟠 手动审查 - 有 Lite 修改的文件，逐行对比合并
🟢 正常 Merge - 接受上游变更
```

---

## 1. 完全删除的文件 🔴

> 这些文件已从 Lite 版本完全删除，上游更新时直接忽略

### 1.1 邮件/验证服务

| 文件 | 状态 |
|------|------|
| `backend/internal/service/email_service.go` | DELETED |
| `backend/internal/service/email_queue_service.go` | DELETED |
| `backend/internal/service/turnstile_service.go` | DELETED |
| `backend/internal/service/identity_service.go` | DELETED |

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

### 1.5 其他删除

| 文件 | 状态 | 说明 |
|------|------|------|
| `backend/internal/handler/auth_linuxdo_oauth.go` | DELETED | LinuxDO OAuth |
| `backend/internal/handler/subscription_handler.go` | DELETED | 用户端订阅 (由 Admin 管理) |
| `backend/internal/service/ops_alert_evaluator_service.go` | DELETED | 依赖 EmailService |
| `backend/internal/service/ops_scheduled_report_service.go` | DELETED | 依赖 EmailService |

---

## 2. 简化保留的文件 🟡

> 这些文件保留但大幅简化，有 [LITE] 标记，merge 时保留 Lite 版本

### 2.1 handler/auth_handler.go

```
状态: SIMPLIFIED
保留功能: Login, GetCurrentUser
删除功能: Register, SendVerifyCode, ValidatePromoCode, LinuxDO OAuth
```

### 2.2 handler/user_handler.go

```
状态: SIMPLIFIED
保留功能: GetProfile, ChangePassword, UpdateProfile
```

### 2.3 handler/api_key_handler.go

```
状态: SIMPLIFIED
保留功能: 完整 CRUD (List, GetByID, Create, Update, Delete)
```

### 2.4 handler/usage_handler.go

```
状态: SIMPLIFIED
保留功能: 完整功能 (List, GetByID, Stats, Dashboard*)
```

### 2.5 service/auth_service.go

```
状态: SIMPLIFIED
保留功能: Login, ValidatePassword, GetUserByID
删除功能: Register, SendVerifyCode, ValidatePromoCode
```

### 2.6 routes/auth.go

```
状态: SIMPLIFIED
保留路由:
  - POST /auth/login
  - GET /settings/public
  - GET /auth/me (需认证)
删除路由: register, send-verify-code, validate-promo-code, oauth/*
```

### 2.7 routes/user.go

```
状态: SIMPLIFIED
保留路由:
  - /user/profile, /user/password, /user
  - /user/totp/* (TOTP 2FA)
  - /keys/* (API Key CRUD)
  - /groups/available
  - /usage/*
删除路由: /redeem/*, /subscriptions/*
```

### 2.8 admin/user_handler.go

```
状态: SIMPLIFIED
保留功能: 用户列表、详情、创建、更新、删除、余额操作
删除功能: 用户属性相关
```

---

## 3. 修改的文件 🟠

> 这些文件有 Lite 专属修改，merge 时需要手动审查

### 3.1 config/config.go

```
状态: MODIFIED
关键修改: 移除 RunModeSimple，只保留 standard 模式
```

### 3.2 billing_cache_service.go

```
状态: MODIFIED
关键函数: checkAPIKeyLimitEligibility
```

**Lite 逻辑**:
```go
// 检查 API Key 限额 -> Group 限额 -> 无限制通过
// 不检查用户余额
```

### 3.3 gateway_service.go

```
状态: MODIFIED
关键函数: RecordUsage, listSchedulableAccounts
```

**Lite 逻辑**:
```go
// RecordUsage: 标准模式只记录用量，不扣余额
// listSchedulableAccounts: 有 groupID 时查分组内账号，否则查所有
```

### 3.3b openai_gateway_service.go

```
状态: MODIFIED
关键函数: listSchedulableAccounts
```

**Lite 逻辑**:
```go
// 传真实 groupID 给 snapshot/repo，不传 nil
// ⚠️ v0.1.109 修复：传 nil 会导致 snapshot 走 ListSchedulableUngroupedByPlatform
//    排除所有已分组的 OpenAI 账号
```

### 3.4 gateway_handler.go

```
状态: MODIFIED
关键函数: Usage
```

**Lite 逻辑**: 返回限额信息而不是余额

### 3.5 middleware/api_key_auth.go & api_key_auth_google.go

```
状态: MODIFIED
关键修改: 移除标准模式的余额检查
```

### 3.6 ent/schema/api_key.go

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

### 3.7 ent/schema/user.go & group.go

```
状态: MODIFIED
关键修改: 移除已删除 schema 的 Edge 引用
```

**user.go 删除的 Edge**:
- redeem_codes, promo_code_usages, attribute_values, allowed_groups

**group.go 删除的 Edge**:
- redeem_codes, allowed_users

### 3.8 Wire 依赖注入

```
状态: MODIFIED
文件: service/wire.go, cmd/server/wire.go, handler/wire.go
```

**Lite 逻辑**: 移除已删除服务的 Provider 和引用

---

## 4. Lite 新增/恢复文件 🟢

### 4.1 后端新增

| 文件 | 说明 |
|------|------|
| `backend/internal/service/api_key_usage_service.go` | API Key 用量追踪服务 |
| `backend/migrations/100_lite_mode.sql` | Lite 模式数据库迁移 |
| `backend/migrations/101_add_user_allowed_groups_id.sql` | user_allowed_groups 添加 id 列 |
| `backend/internal/repository/api_key_usage_repo.go` | API Key 用量 Repository |
| `frontend/src/api/auth.ts` | Lite 版 Auth API (login/logout/getCurrentUser) |

### 4.1b 后端新增 — Active Hours (v0.1.88-lite)

| 文件 | 说明 |
|------|------|
| `backend/internal/service/account.go` | 新增 ActiveHourRule/GetActiveHours/isInActiveWindow |
| `backend/internal/handler/dto/types.go` | 新增 ActiveHourRule DTO |
| `backend/internal/handler/dto/mappers.go` | active_hours 映射到 DTO |
| `frontend/src/components/account/EditAccountModal.vue` | 活跃时间段 UI |
| `frontend/src/components/account/CreateAccountModal.vue` | 创建时支持配置 |

### 4.1c 部署脚本修复 (v0.1.88-lite)

| 文件 | 说明 |
|------|------|
| `deploy/docker-deploy.sh` | GitHub URL 改为 Kevin-Lab777/Light，下载正确的 compose 文件 |
| `deploy/docker-compose.yml` | 支持 named volumes / 本地目录映射切换（通过 .env） |
| `deploy/.env.example` | 新增 DATA_PATH/POSTGRES_DATA_PATH/REDIS_DATA_PATH |
| `deploy/docker-compose.local.yml` | **已删除**（使用上游镜像，非 Lite） |

### 4.2 前端恢复（用户 Profile 功能）

> v0.1.53-lite 恢复了用户个人资料页面，支持修改密码

| 文件 | 说明 |
|------|------|
| `frontend/src/api/user.ts` | 用户 API (getProfile, updateProfile, changePassword) |
| `frontend/src/views/user/ProfileView.vue` | 个人资料页面 |
| `frontend/src/components/user/profile/ProfileInfoCard.vue` | 用户信息卡片组件 |
| `frontend/src/components/user/profile/ProfileEditForm.vue` | 编辑用户名表单组件 |
| `frontend/src/components/user/profile/ProfilePasswordForm.vue` | 修改密码表单组件 |

### 4.3 前端修改

| 文件 | 修改内容 |
|------|----------|
| `frontend/src/api/index.ts` | 导出 userAPI |
| `frontend/src/router/index.ts` | 添加 `/profile` 路由 |
| `frontend/src/components/layout/AppSidebar.vue` | 底部添加个人资料入口 |
| `frontend/src/components/layout/AppHeader.vue` | 余额显示改为今日使用额度 |
| `frontend/src/i18n/locales/zh.ts` | 添加 `todayUsage` 翻译 |
| `frontend/src/i18n/locales/en.ts` | 添加 `todayUsage` 翻译 |

---

## 5. 正常 Merge 文件 🟢

以下文件/目录可以直接接受上游更新：

```
backend/internal/handler/admin/account_handler.go
backend/internal/handler/admin/proxy_handler.go
backend/internal/handler/admin/group_handler.go
backend/internal/handler/admin/dashboard_handler.go
backend/internal/handler/admin/setting_handler.go
backend/internal/handler/admin/system_handler.go
backend/internal/handler/admin/subscription_handler.go
backend/internal/handler/admin/usage_handler.go
backend/internal/handler/admin/*_oauth_handler.go
backend/internal/handler/admin/ops_*.go  # v0.1.50 新增

backend/internal/service/account*.go
backend/internal/service/proxy*.go
backend/internal/service/group*.go
backend/internal/service/dashboard_service.go
backend/internal/service/setting*.go
backend/internal/service/subscription_service.go
backend/internal/service/usage*.go
backend/internal/service/*_oauth_service.go
backend/internal/service/ops_*.go  # v0.1.50 新增 (除 alert/scheduled)

backend/internal/middleware/
backend/internal/pkg/
backend/migrations/  # 我们的从 100 开始，不冲突
backend/internal/repository/ops_*.go  # v0.1.50 新增

frontend/  # 需要检查是否有冲突
```

---

## 6. 版本历史

| 版本 | 日期 | 基于上游 | 说明 |
|------|------|----------|------|
| v0.1.35-lite.1 | 2026-01-10 | v0.1.35 | 初始 Lite 版本 |
| v0.1.46-lite | 2026-01-11 | v0.1.46 | 同步上游更新 |
| v0.1.50-lite | 2026-01-12 | v0.1.50 | 同步 Ops 监控系统 |
| v0.1.53-lite | 2026-01-13 | v0.1.53 | 恢复用户 Profile 页面，支持修改密码 |
| v0.1.76-lite | 2026-02-17 | v0.1.76 | 同步上游 TOTP/公告/错误透传/分组排序等功能 |
| v0.1.83-lite | 2026-02-19 | v0.1.83 | 同步 Cache TTL Override/Codex 修复/Sonnet 4.6 等 |
| v0.1.88-lite | 2026-03-16 | v0.1.88 | 同步 Sora/IP白名单/API Key quota+rate_limit/幂等性等 + 修复计费/账号调度/TOTP/前端精简 |
| v0.1.103-lite | 2026-03-28 | v0.1.103 | 同步 DB备份/Backend Mode/端点可观测性/upstream model追踪/分组容量列/529 cooldown等 |
| v0.1.109-lite | 2026-04-08 | v0.1.109 | 同步 Channel定价/Sora完全移除/gpt-5.4-mini/虚拟滚动/requested model追踪等 |
