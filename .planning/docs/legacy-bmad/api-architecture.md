# Sub2API 系统架构文档

> 生成日期: 2026-01-10
> 项目名称: AIRelayServices/sub2api

---

## 1. 系统概述

Sub2API 是一个 **AI API 网关/代理系统**，核心功能是将多个 AI 平台账号（Claude、Gemini、OpenAI、Antigravity）聚合为统一的 API 接口，提供：

- **多账号负载均衡** - 智能调度多个上游账号
- **用户管理与计费** - 余额/订阅两种计费模式
- **并发控制** - 用户级和账号级并发限制
- **API Key 管理** - 用户可创建多个 API Key
- **分组机制** - 灵活的账号分组与权限控制

---

## 2. 技术栈

| 层级 | 技术 |
|------|------|
| 后端框架 | Go + Gin |
| ORM | Ent (Facebook) |
| 数据库 | PostgreSQL |
| 缓存 | Redis |
| 依赖注入 | Wire |
| 前端 | Vue 3 + TypeScript + Vite |

---

## 3. 项目结构

```
sub2api/
├── backend/
│   ├── cmd/server/          # 入口点
│   ├── ent/                  # 数据库 Schema (Ent ORM)
│   │   └── schema/           # 实体定义
│   ├── internal/
│   │   ├── config/           # 配置管理
│   │   ├── handler/          # HTTP Handler 层
│   │   │   ├── admin/        # 管理后台 Handler
│   │   │   └── dto/          # 数据传输对象
│   │   ├── middleware/       # 中间件
│   │   ├── pkg/              # 内部工具包
│   │   ├── repository/       # 数据访问层
│   │   ├── server/           # HTTP 服务器
│   │   │   ├── middleware/   # 服务器中间件
│   │   │   └── routes/       # 路由注册
│   │   └── service/          # 业务逻辑层
│   └── migrations/           # 数据库迁移
└── frontend/
    └── src/
        ├── api/              # API 客户端
        ├── components/       # Vue 组件
        ├── composables/      # 组合式函数
        ├── stores/           # Pinia 状态管理
        └── views/            # 页面视图
```

---

## 4. 数据模型

### 4.1 核心实体

```
┌─────────────┐       ┌─────────────┐       ┌─────────────┐
│    User     │──1:N──│   APIKey    │──N:1──│    Group    │
├─────────────┤       ├─────────────┤       ├─────────────┤
│ id          │       │ id          │       │ id          │
│ email       │       │ user_id     │       │ name        │
│ password    │       │ key         │       │ platform    │
│ role        │       │ name        │       │ billing_type│
│ balance     │       │ group_id    │       │ daily_limit │
│ concurrency │       │ status      │       │ weekly_limit│
│ status      │       │ ip_whitelist│       │ monthly_limit│
└─────────────┘       └─────────────┘       └─────────────┘
                                                   │
                                                   │ N:M
                                                   ▼
┌─────────────┐       ┌─────────────┐       ┌─────────────┐
│   Account   │──N:M──│AccountGroup │──────│    Group    │
├─────────────┤       └─────────────┘       └─────────────┘
│ id          │
│ name        │       ┌─────────────┐
│ platform    │──N:1──│    Proxy    │
│ type        │       ├─────────────┤
│ credentials │       │ id          │
│ concurrency │       │ name        │
│ priority    │       │ host        │
│ status      │       │ port        │
│ schedulable │       │ username    │
│ rate_limited│       │ password    │
└─────────────┘       └─────────────┘
```

### 4.2 支持的平台

| Platform | 类型 | 说明 |
|----------|------|------|
| `claude` | api_key / oauth / cookie | Anthropic Claude API |
| `gemini` | oauth | Google Gemini (OAuth 流程) |
| `openai` | oauth | OpenAI API (OAuth 流程) |
| `antigravity` | oauth | Antigravity 平台 |

### 4.3 订阅模型

```
┌─────────────────┐       ┌─────────────────┐
│      User       │──1:N──│ UserSubscription│
└─────────────────┘       ├─────────────────┤
                          │ group_id        │
                          │ starts_at       │
                          │ expires_at      │
                          │ daily_usage_usd │
                          │ weekly_usage_usd│
                          │ monthly_usage_usd│
                          └─────────────────┘
```

---

## 5. API 路由架构

### 5.1 路由层次结构

```
/
├── /health                          # 健康检查
├── /setup/status                    # 初始化状态
│
├── /api/v1/                         # REST API v1
│   ├── /auth/                       # 认证模块 (公开)
│   ├── /settings/public             # 公开设置
│   ├── /user/                       # 用户模块 (需 JWT)
│   ├── /keys/                       # API Key 管理 (需 JWT)
│   ├── /groups/                     # 分组查询 (需 JWT)
│   ├── /usage/                      # 使用记录 (需 JWT)
│   ├── /redeem/                     # 卡密兑换 (需 JWT)
│   ├── /subscriptions/              # 订阅管理 (需 JWT)
│   └── /admin/                      # 管理后台 (需 Admin)
│
├── /v1/                             # Claude API 兼容 (需 API Key)
│   ├── POST /messages               # 消息接口
│   ├── POST /messages/count_tokens  # Token 计数
│   ├── GET  /models                 # 模型列表
│   ├── GET  /usage                  # 使用量查询
│   └── POST /responses              # OpenAI Responses API
│
├── /v1beta/                         # Gemini 原生 API (需 API Key)
│   ├── GET  /models                 # 模型列表
│   ├── GET  /models/:model          # 模型详情
│   └── POST /models/*modelAction    # 模型操作
│
├── /antigravity/                    # Antigravity 专用路由
│   ├── GET  /models                 # 模型列表
│   ├── /v1/                         # Claude 兼容 (强制 Antigravity 账号)
│   └── /v1beta/                     # Gemini 兼容 (强制 Antigravity 账号)
│
└── /responses                       # OpenAI Responses (别名)
```

---

## 6. API 详细清单

### 6.1 认证模块 `/api/v1/auth`

| 方法 | 路径 | Handler | 说明 |
|------|------|---------|------|
| POST | `/register` | Auth.Register | 用户注册 |
| POST | `/login` | Auth.Login | 用户登录 |
| POST | `/send-verify-code` | Auth.SendVerifyCode | 发送验证码 |
| POST | `/validate-promo-code` | Auth.ValidatePromoCode | 验证优惠码 (限流) |
| GET | `/oauth/linuxdo/start` | Auth.LinuxDoOAuthStart | LinuxDO OAuth 开始 |
| GET | `/oauth/linuxdo/callback` | Auth.LinuxDoOAuthCallback | LinuxDO OAuth 回调 |
| GET | `/me` | Auth.GetCurrentUser | 获取当前用户 (需 JWT) |

### 6.2 用户模块 `/api/v1/user`

| 方法 | 路径 | Handler | 说明 |
|------|------|---------|------|
| GET | `/profile` | User.GetProfile | 获取个人信息 |
| PUT | `/password` | User.ChangePassword | 修改密码 |
| PUT | `/` | User.UpdateProfile | 更新个人信息 |

### 6.3 API Key 管理 `/api/v1/keys`

| 方法 | 路径 | Handler | 说明 |
|------|------|---------|------|
| GET | `/` | APIKey.List | 列表 |
| GET | `/:id` | APIKey.GetByID | 详情 |
| POST | `/` | APIKey.Create | 创建 |
| PUT | `/:id` | APIKey.Update | 更新 |
| DELETE | `/:id` | APIKey.Delete | 删除 |

### 6.4 使用记录 `/api/v1/usage`

| 方法 | 路径 | Handler | 说明 |
|------|------|---------|------|
| GET | `/` | Usage.List | 列表 |
| GET | `/:id` | Usage.GetByID | 详情 |
| GET | `/stats` | Usage.Stats | 统计 |
| GET | `/dashboard/stats` | Usage.DashboardStats | 仪表盘统计 |
| GET | `/dashboard/trend` | Usage.DashboardTrend | 趋势图 |
| GET | `/dashboard/models` | Usage.DashboardModels | 模型统计 |
| POST | `/dashboard/api-keys-usage` | Usage.DashboardAPIKeysUsage | API Key 使用量 |

### 6.5 订阅管理 `/api/v1/subscriptions`

| 方法 | 路径 | Handler | 说明 |
|------|------|---------|------|
| GET | `/` | Subscription.List | 列表 |
| GET | `/active` | Subscription.GetActive | 活跃订阅 |
| GET | `/progress` | Subscription.GetProgress | 进度 |
| GET | `/summary` | Subscription.GetSummary | 摘要 |

### 6.6 卡密兑换 `/api/v1/redeem`

| 方法 | 路径 | Handler | 说明 |
|------|------|---------|------|
| POST | `/` | Redeem.Redeem | 兑换卡密 |
| GET | `/history` | Redeem.GetHistory | 兑换历史 |

---

## 7. 管理后台 API `/api/v1/admin`

### 7.1 仪表盘 `/dashboard`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/stats` | 总体统计 |
| GET | `/realtime` | 实时指标 |
| GET | `/trend` | 使用趋势 |
| GET | `/models` | 模型统计 |
| GET | `/api-keys-trend` | API Key 趋势 |
| GET | `/users-trend` | 用户趋势 |
| POST | `/users-usage` | 批量用户使用量 |
| POST | `/api-keys-usage` | 批量 API Key 使用量 |

### 7.2 用户管理 `/users`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` | 用户列表 |
| GET | `/:id` | 用户详情 |
| POST | `/` | 创建用户 |
| PUT | `/:id` | 更新用户 |
| DELETE | `/:id` | 删除用户 |
| POST | `/:id/balance` | 更新余额 |
| GET | `/:id/api-keys` | 用户 API Keys |
| GET | `/:id/usage` | 用户使用量 |
| GET | `/:id/attributes` | 用户属性 |
| PUT | `/:id/attributes` | 更新用户属性 |

### 7.3 分组管理 `/groups`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` | 分页列表 |
| GET | `/all` | 全部分组 |
| GET | `/:id` | 分组详情 |
| POST | `/` | 创建分组 |
| PUT | `/:id` | 更新分组 |
| DELETE | `/:id` | 删除分组 |
| GET | `/:id/stats` | 分组统计 |
| GET | `/:id/api-keys` | 分组 API Keys |

### 7.4 账号管理 `/accounts`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` | 账号列表 |
| GET | `/:id` | 账号详情 |
| POST | `/` | 创建账号 |
| PUT | `/:id` | 更新账号 |
| DELETE | `/:id` | 删除账号 |
| POST | `/:id/test` | 测试账号 |
| POST | `/:id/refresh` | 刷新 Token |
| POST | `/:id/refresh-tier` | 刷新 Tier |
| GET | `/:id/stats` | 账号统计 |
| POST | `/:id/clear-error` | 清除错误 |
| GET | `/:id/usage` | 账号使用量 |
| GET | `/:id/today-stats` | 今日统计 |
| POST | `/:id/clear-rate-limit` | 清除限流 |
| GET | `/:id/temp-unschedulable` | 临时不可调度 |
| DELETE | `/:id/temp-unschedulable` | 清除临时不可调度 |
| POST | `/:id/schedulable` | 设置可调度 |
| GET | `/:id/models` | 可用模型 |
| POST | `/batch` | 批量创建 |
| POST | `/batch-update-credentials` | 批量更新凭证 |
| POST | `/batch-refresh-tier` | 批量刷新 Tier |
| POST | `/bulk-update` | 批量更新 |
| POST | `/sync/crs` | 从 CRS 同步 |

### 7.5 OAuth 集成

**Claude OAuth `/accounts`**
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/generate-auth-url` | 生成授权 URL |
| POST | `/generate-setup-token-url` | 生成 Setup Token URL |
| POST | `/exchange-code` | 交换授权码 |
| POST | `/exchange-setup-token-code` | 交换 Setup Token 码 |
| POST | `/cookie-auth` | Cookie 认证 |
| POST | `/setup-token-cookie-auth` | Setup Token Cookie 认证 |

**OpenAI OAuth `/openai`**
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/generate-auth-url` | 生成授权 URL |
| POST | `/exchange-code` | 交换授权码 |
| POST | `/refresh-token` | 刷新 Token |
| POST | `/accounts/:id/refresh` | 刷新账号 Token |
| POST | `/create-from-oauth` | OAuth 创建账号 |

**Gemini OAuth `/gemini`**
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/oauth/auth-url` | 生成授权 URL |
| POST | `/oauth/exchange-code` | 交换授权码 |
| GET | `/oauth/capabilities` | 获取能力 |

**Antigravity OAuth `/antigravity`**
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/oauth/auth-url` | 生成授权 URL |
| POST | `/oauth/exchange-code` | 交换授权码 |

### 7.6 代理管理 `/proxies`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` | 代理列表 |
| GET | `/all` | 全部代理 |
| GET | `/:id` | 代理详情 |
| POST | `/` | 创建代理 |
| PUT | `/:id` | 更新代理 |
| DELETE | `/:id` | 删除代理 |
| POST | `/:id/test` | 测试代理 |
| GET | `/:id/stats` | 代理统计 |
| GET | `/:id/accounts` | 代理账号 |
| POST | `/batch` | 批量创建 |

### 7.7 卡密管理 `/redeem-codes`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` | 卡密列表 |
| GET | `/stats` | 卡密统计 |
| GET | `/export` | 导出卡密 |
| GET | `/:id` | 卡密详情 |
| POST | `/generate` | 生成卡密 |
| DELETE | `/:id` | 删除卡密 |
| POST | `/batch-delete` | 批量删除 |
| POST | `/:id/expire` | 使卡密过期 |

### 7.8 优惠码管理 `/promo-codes`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` | 优惠码列表 |
| GET | `/:id` | 优惠码详情 |
| POST | `/` | 创建优惠码 |
| PUT | `/:id` | 更新优惠码 |
| DELETE | `/:id` | 删除优惠码 |
| GET | `/:id/usages` | 使用记录 |

### 7.9 系统设置 `/settings`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` | 获取设置 |
| PUT | `/` | 更新设置 |
| POST | `/test-smtp` | 测试 SMTP |
| POST | `/send-test-email` | 发送测试邮件 |
| GET | `/admin-api-key` | 获取管理 API Key |
| POST | `/admin-api-key/regenerate` | 重新生成管理 API Key |
| DELETE | `/admin-api-key` | 删除管理 API Key |

### 7.10 系统管理 `/system`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/version` | 版本信息 |
| GET | `/check-updates` | 检查更新 |
| POST | `/update` | 执行更新 |
| POST | `/rollback` | 回滚 |
| POST | `/restart` | 重启服务 |

### 7.11 订阅管理 (Admin) `/subscriptions`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` | 订阅列表 |
| GET | `/:id` | 订阅详情 |
| GET | `/:id/progress` | 订阅进度 |
| POST | `/assign` | 分配订阅 |
| POST | `/bulk-assign` | 批量分配 |
| POST | `/:id/extend` | 延长订阅 |
| DELETE | `/:id` | 撤销订阅 |

### 7.12 使用记录 (Admin) `/usage`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` | 使用列表 |
| GET | `/stats` | 统计 |
| GET | `/search-users` | 搜索用户 |
| GET | `/search-api-keys` | 搜索 API Keys |

### 7.13 用户属性 `/user-attributes`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` | 属性定义列表 |
| POST | `/` | 创建属性定义 |
| POST | `/batch` | 批量获取用户属性 |
| PUT | `/reorder` | 重新排序 |
| PUT | `/:id` | 更新属性定义 |
| DELETE | `/:id` | 删除属性定义 |

---

## 8. Gateway API (AI 代理)

### 8.1 Claude 兼容 `/v1`

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/messages` | 消息接口 (核心) |
| POST | `/messages/count_tokens` | Token 计数 |
| GET | `/models` | 可用模型列表 |
| GET | `/usage` | 余额/配额查询 |
| POST | `/responses` | OpenAI Responses API |

### 8.2 Gemini 原生 `/v1beta`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/models` | 模型列表 |
| GET | `/models/:model` | 模型详情 |
| POST | `/models/*modelAction` | 模型操作 (generateContent 等) |

### 8.3 Antigravity `/antigravity`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/models` | Antigravity 模型列表 |
| `/v1/*` | | Claude 兼容 (强制 Antigravity) |
| `/v1beta/*` | | Gemini 兼容 (强制 Antigravity) |

---

## 9. 核心业务流程

### 9.1 请求处理流程

```
                                   ┌──────────────────────┐
                                   │    API Key Auth      │
                                   │    Middleware        │
                                   └──────────┬───────────┘
                                              │
                                   ┌──────────▼───────────┐
                                   │  Billing Eligibility │
                                   │  Check (余额/订阅)    │
                                   └──────────┬───────────┘
                                              │
                                   ┌──────────▼───────────┐
                                   │   User Concurrency   │
                                   │   Slot Acquire       │
                                   └──────────┬───────────┘
                                              │
                                   ┌──────────▼───────────┐
                                   │   Account Selection  │
                                   │   (Load Balancing)   │
                                   └──────────┬───────────┘
                                              │
                                   ┌──────────▼───────────┐
                                   │  Account Concurrency │
                                   │  Slot Acquire        │
                                   └──────────┬───────────┘
                                              │
                              ┌───────────────┼───────────────┐
                              │               │               │
                     ┌────────▼────────┐ ┌────▼────┐ ┌────────▼────────┐
                     │ Claude Forward  │ │ Gemini  │ │ Antigravity     │
                     │                 │ │ Forward │ │ Forward         │
                     └────────┬────────┘ └────┬────┘ └────────┬────────┘
                              │               │               │
                              └───────────────┼───────────────┘
                                              │
                                   ┌──────────▼───────────┐
                                   │   Usage Recording    │
                                   │   (Async)            │
                                   └──────────────────────┘
```

### 9.2 账号选择策略

1. **分组过滤** - 根据 API Key 的 group_id 筛选账号
2. **模型匹配** - 检查账号的 model_mapping 白名单
3. **状态检查** - schedulable=true, status=active
4. **速率限制** - 排除 rate_limited 账号
5. **并发检查** - 优先选择低负载账号
6. **粘性会话** - 相同会话尽量复用同一账号
7. **优先级排序** - 按 priority 排序

### 9.3 计费模式

| 模式 | 说明 | 扣费方式 |
|------|------|----------|
| 余额模式 | 用户钱包扣费 | 按 Token 用量计费 |
| 订阅模式 | 分组订阅制 | 日/周/月额度限制 |

---

## 10. 二次开发切入点

### 10.1 新增 API 端点

1. **路由注册**: `backend/internal/server/routes/`
2. **Handler**: `backend/internal/handler/`
3. **Service**: `backend/internal/service/`
4. **Repository**: `backend/internal/repository/`

### 10.2 新增平台支持

1. 在 `service/domain_constants.go` 添加平台常量
2. 创建 `*_gateway_service.go` 实现转发逻辑
3. 在 `gateway_handler.go` 添加平台路由分支
4. 更新 `ent/schema/account.go` 支持新凭证类型

### 10.3 扩展数据模型

1. 在 `ent/schema/` 创建新的 Schema 文件
2. 运行 `go generate ./ent` 生成代码
3. 创建迁移文件 `migrations/xxx_*.sql`

### 10.4 中间件扩展

位置: `backend/internal/server/middleware/` 或 `backend/internal/middleware/`

### 10.5 前端扩展

1. **API 客户端**: `frontend/src/api/`
2. **页面**: `frontend/src/views/`
3. **组件**: `frontend/src/components/`
4. **状态管理**: `frontend/src/stores/`

---

## 11. 关键配置

配置文件: `config.yaml`

```yaml
# 主要配置项
server:
  port: 8080

database:
  host: localhost
  port: 5432
  name: sub2api

redis:
  host: localhost
  port: 6379

gateway:
  max_body_size: 10MB

concurrency:
  ping_interval: 30

cors:
  allowed_origins: ["*"]

security:
  csp: "default-src 'self'"
```

---

## 12. 总结

Sub2API 是一个成熟的 AI API 代理系统，具有以下特点：

- **架构清晰**: 标准的分层架构 (Handler → Service → Repository)
- **功能完善**: 多平台支持、负载均衡、计费系统、权限管理
- **可扩展性好**: 模块化设计，易于添加新平台和功能
- **技术栈现代**: Go + Gin + Ent + Vue 3

二次开发时，建议：
1. 先熟悉核心的 Gateway 请求流程
2. 理解账号调度和并发控制机制
3. 根据需求选择合适的扩展点
