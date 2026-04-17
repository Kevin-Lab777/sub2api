# Sub2API Lite Release Notes

> 所有版本的发布记录

---

## v0.1.109-lite (2026-04-09)

> 基于上游 [Wei-Shaw/sub2api v0.1.109](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.109)
>
> 上游 v0.1.103 → v0.1.109，236 non-merge commits

### Lite 专属改动

- 🐛 **修复 OpenAI 账号调度** - `listSchedulableAccounts` 传 `nil` groupID 导致已分组的 OpenAI 账号被排除，`/v1/responses` 报 503
- 🔧 **ProvideOAuthRefreshAPI** - 新增 wire-compatible wrapper 解决 variadic 参数问题
- 🔧 **wire.go 语法修复** - 修复 OpsSystemLogSink 后残留闭合括号
- 🔧 **README_JA.md URL 替换** - 上游新增日语 README，Wei-Shaw → Kevin-Lab777/Light
- 🔧 **前端 DataManagement 清理** - 移除不存在的 DataManagementView 路由和导航引用
- 🔧 **删除 identity_service_order_test.go** - 上游新增测试依赖已删除服务

### 合并上游核心功能

#### Sora 平台完全移除 (v0.1.107)
- 🗑️ **完全移除 Sora 功能** - schema/handler/service/前端页面/S3 存储全部删除
- 🗑️ **DataManagementView 清理** - Sora S3 配置移除后合并到 BackupView

#### Channel 定价系统
- ✨ **Channel Token Pricing** - channel 定价优先于 per-image billing
- ✨ **Model Restriction** - 从 handler 移至 scheduling phase
- ✨ **Cache Fail-Close** - channel cache 失败时关闭策略
- ✨ **定价 UI 重构** - 提取 formatCacheTokens/formatMultiplier 到共享工具

#### 模型支持
- ✨ **gpt-5.4-mini/nano** - 新模型支持与定价配置
- ✨ **Beta 策略模型白名单** - 按模型区分处理
- ✨ **max_claude_code_version** - 控制 Claude Code 版本设置

#### 使用记录增强
- ✨ **Requested Model 追踪** - usage_log 新增 requested_model 字段和迁移
- 🐛 **配额显示修复** - 每日/每周重置后不再显示过期累积用量

#### 前端优化
- ⚡ **虚拟滚动** - DataTable 使用 @tanstack/vue-virtual 提升大表性能
- ✨ **分页持久化** - pageSize 保存到 localStorage
- ✨ **用户管理增强** - 新增分组列/分组筛选/专属分组一键替换

#### Bug Fixes
- 🐛 非流式路径上游终态事件 output 为空时从 delta 事件重建响应内容
- 🐛 非 Claude Code 客户端 system prompt 迁移至 messages
- 🐛 billing mode display follows cost calculation result
- 🐛 channel_mapped override 不再错误回退 BillingModel
- 🐛 refresh token race condition 修复
- 🐛 Antigravity credits exhausted 429 正确标记
- 🐛 流式 failover 记录原始 upstream status code

**Full Changelog**: https://github.com/Kevin-Lab777/sub2api/compare/v0.1.103-lite...v0.1.109-lite

---

## v0.1.103-lite (2026-03-28)

> 基于上游 [Wei-Shaw/sub2api v0.1.103](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.103)
>
> 上游 v0.1.88 → v0.1.103，116 non-merge commits

### Lite 专属改动

- 🔧 **Import 路径替换** - 所有上游新增文件 Wei-Shaw → Kevin-Lab777
- 🔧 **wire_gen.go 重构** - 保持 Lite 依赖注入精简（移除已删除服务）
- 🔧 **URL/品牌替换** - README/Dockerfile/前端 GitHub 链接更新
- 🔧 **前端 DatabaseIcon** - AppSidebar 新增备份功能图标
- 🔧 **fingerprint UserAgent** - gateway_service.go fingerprint struct 新增 UserAgent 字段

### 合并上游核心功能

#### 备份与数据管理
- ✨ **数据库定时备份与恢复** - S3 兼容存储，支持 Cloudflare R2
- ✨ **备份管理 UI 整合** - 备份和数据管理合并为设置页标签页

#### Backend Mode & 设置
- ✨ **Backend Mode** - 禁用用户自助服务的切换开关
- ✨ **529 Overload Cooldown** - 可配置开关和持续时间
- 🐛 **config.yaml 只读挂载** - 兼容只读方式挂载配置文件

#### 端点可观测性 & Dashboard
- ✨ **统一端点标准化** - 所有 handler 迁移至 endpoint normalization 中间件
- ✨ **Upstream Model 追踪** - 所有 gateway 路径记录上游实际使用模型
- ✨ **用量端点字段** - usage_log 新增 InboundEndpoint/UpstreamEndpoint
- ✨ **Dashboard 用户细分** - per-user drill-down（分组/模型/端点分布）
- ✨ **Model Source 过滤** - Dashboard 统计支持 model_source 维度

#### 账号管理增强
- ✨ **配额固定时间重置** - 账号配额支持固定时间重置模式
- ✨ **分组容量列** - 并发/会话/RPM 实时聚合展示
- ✨ **分组用量与账号分类** - 分组列表新增用量列和账号数分类

#### Antigravity & AI Credits
- ✨ **AI Credits 余额处理** - 增强 overages 处理和 UI 展示
- ✨ **403 Forbidden 检测** - Antigravity 新增 403 状态检测和分类
- ✨ **统一 OAuth Refresh** - 分布式锁 + 统一 token 刷新 API

#### 平台 & 模型
- ✨ **Bedrock 统一** - bedrock-apikey 合并为 bedrock with auth_mode
- ✨ **Haiku→Sonnet 映射** - claude-haiku-4-5 variants 映射到 claude-sonnet-4-6
- ✨ **OpenAI 模型映射** - admin 可用模型尊重 account model mapping
- ✨ **Claude Effort 日志** - 记录 output_config.effort

#### 订阅管理
- ✨ **教程指南弹窗** - 订阅管理页面添加指引
- ✨ **平台类型过滤** - 订阅管理支持按平台筛选

#### Bug Fixes
- 🐛 **流式 failover 修复** - 防止拼接腐化导致双 message_start
- 🐛 **Bedrock 配额修复** - 配额计数器正确计算
- 🐛 **批量编辑模型白名单** - 修复显示与实际不一致
- 🐛 **Group quota 清除** - 允许清除限额并正确处理 0 值
- 🐛 **Rate limit window** - nil window 视为已过期防止用量累积
- 🐛 **DST 安全查询** - 使用 half-open date ranges
- 🐛 **Claude Code 兼容** - v2.1.78+ 新 JSON 格式 metadata.user_id
- 🐛 **站点标题注入** - 直接访问页面时显示自定义站点名称
- 🐛 **个人资料弹窗** - GitHub 链接仅对管理员可见

### ⚠️ Lite 不可用功能
- `ops_alert_evaluator_service` - 告警邮件通知（依赖 EmailService）
- `ops_scheduled_report_service` - 定时报告邮件（依赖 EmailService）
- 邀请码注册功能（依赖用户注册系统）
- 密码重置邮件（依赖 EmailService）
- 优惠码功能（已删除）
- 兑换码功能（已删除）

**Full Changelog**: https://github.com/Kevin-Lab777/sub2api/compare/v0.1.88-lite...v0.1.103-lite

---

## v0.1.88-lite (2026-03-16)

> 基于上游 [Wei-Shaw/sub2api v0.1.88](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.88)
>
> 上游 v0.1.83 → v0.1.88，856 files changed (+188k/-49k)，大规模重构

### Lite 专属改动

- ✨ **Active Hours 账号活跃时间段** - 控制账号仅在每天指定时间段内参与调度（如 8:00-22:00），支持跨午夜
- 🔧 **user_allowed_group 保留决策** - 上游升级为核心功能（独立 join table + ent schema），不再删除
- 🔧 **DB 迁移 101** - user_allowed_groups 添加 id 自增列（ent 要求）
- 🔧 **前端 auth.ts 重建** - 创建 Lite 版 Auth API（login/logout/getCurrentUser/isTotp2FARequired）
- 🔧 **前端清理** - 移除 redeem/promo/userAttributes 导入、修复 SoraPromptBar subscriptionStore 依赖
- 🔧 **部署脚本修复** - docker-deploy.sh GitHub URL 改为 Lite fork，删除 local.yml
- ✨ **Docker 存储灵活配置** - docker-compose.yml 支持 named volumes / 本地目录映射（通过 .env 切换）
- 🔧 **api_key_usage_repo.go** - 新建 APIKeyUsageRepository 实现
- 🔧 **Ent Schema 修复** - composite key 注解兼容、重新添加 Lite 限额字段
- 🔧 **Wire 修复** - 添加 Sora/IdentityCache/ScheduledTest 等上游新增 Provider

### Merge 后修复 (2026-03-16)

- 🐛 **计费逻辑恢复** - `CheckBillingEligibility` 被上游余额检查覆盖，非订阅模式直接走 `checkAPIKeyLimitEligibility` 不检查余额
- 🐛 **账号调度恢复** - `listSchedulableAccounts` mixed 分支被上游 `RunModeSimple` 条件覆盖，Lite 无法获取分组内账号
- 🐛 **TOTP 2FA 路由注册** - `routes/user.go` 漏掉 `/user/totp/*` 路由组，导致前端 2FA 操作 404
- 🎨 **设置页精简** - 删除注册/邀请码/优惠码/Turnstile/LinuxDO 设置项，保留邮箱验证+密码重置+TOTP
- 🎨 **分组计费类型** - 重命名为 "Lite 模式（无余额限制）"，standard 分组也可设限额字段

### 合并上游核心功能

#### Sora 视频生成
- ✨ **Sora SDK 集成** - 视频生成 API + 媒体存储 + S3 集成
- ✨ **Sora 配额管理** - Generation Service + Quota Service
- ✨ **Sora S3 多 Profile** - 支持多 S3 存储配置

#### API Key 增强
- ✨ **IP 白名单/黑名单** - 每个 API Key 可配置 IP/CIDR 访问控制
- ✨ **quota/rate_limit 体系** - 5h/1d/7d 窗口速率限制
- ✨ **API Key 过期时间** - expires_at 字段
- ✨ **API Key last_used_at** - 最后使用时间追踪

#### 可靠性 & 性能
- ✨ **幂等性控制** - IdempotencyCoordinator 防重复请求
- ✨ **定时测试** - ScheduledTestService + 自动恢复
- ✨ **用户消息串行队列** - UserMessageQueueService
- ✨ **Scheduler Snapshot** - 调度器快照服务
- ✨ **延迟服务** - DeferredService
- ✨ **使用计费去重** - usage_billing_dedup 机制

#### 平台支持
- ✨ **Bedrock/AWS 支持** - 新增 AWS Bedrock 平台
- ✨ **OpenAI WebSocket v2** - responses_websockets_v2 协议
- ✨ **Gemini 2.5 Flash** - 图片模型映射

#### 前端
- ✨ **Sora 客户端页面** - 视频生成 UI
- ✨ **自定义菜单** - 可配置侧边栏菜单项
- ✨ **购买订阅页面** - PurchaseSubscriptionView
- ✨ **搜索增强** - trigram 索引 + 模糊搜索
- ✨ **Ops 系统日志** - OpsSystemLogTable 组件

### ⚠️ Lite 不可用功能
- `ops_alert_evaluator_service` - 告警邮件通知（依赖 EmailService）
- `ops_scheduled_report_service` - 定时报告邮件（依赖 EmailService）
- 邀请码注册功能（依赖用户注册系统）
- 密码重置邮件（依赖 EmailService）
- 优惠码功能（已删除）
- 兑换码功能（已删除）

**Full Changelog**: https://github.com/Kevin-Lab777/sub2api/compare/v0.1.83-lite...v0.1.88-lite

---

## v0.1.83-lite (2026-02-19)

> 基于上游 [Wei-Shaw/sub2api v0.1.83](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.83)
>
> 上游 v0.1.76 → v0.1.83，跨 7 commits

### Lite 专属改动

- 🔧 **Ent 重新生成** - 适配 usage_log 新增 `cache_ttl_overridden` 字段
- 🔧 **Wire 重新生成** - 保持依赖注入一致性
- 🗑️ **移除 UsageView.vue** - 上游修改了已删除文件，继续保持删除

### 合并上游功能

#### 账号管理
- ✨ **Cache TTL Override per account** - 管理员可强制替换账号的缓存 TTL 计费策略
- ✨ **新增迁移 055_add_cache_ttl_overridden.sql** - usage_log 表新增 `cache_ttl_overridden` 字段

#### Gateway & 模型
- 🐛 **Codex 参数清理** - 移除 Codex 模型请求中的不支持参数 (strip unsupported params)
- 🐛 **OpenAI CORS 修复** - 接受 `x-stainless-*` header 解决 CORS 错误

#### 前端
- ✨ **Sonnet 4.6 快捷映射按钮** - 前端新增 Claude Sonnet 4.6 模型快捷映射

### ⚠️ Lite 不可用功能
- `ops_alert_evaluator_service` - 告警邮件通知（依赖 EmailService）
- `ops_scheduled_report_service` - 定时报告邮件（依赖 EmailService）
- 邀请码注册功能（依赖用户注册系统）
- 密码重置邮件（依赖 EmailService）
- 优惠码功能（已删除）

**Full Changelog**: https://github.com/Kevin-Lab777/sub2api/compare/v0.1.76-lite...v0.1.83-lite

---

## v0.1.76-lite (2026-02-17)

> 基于上游 [Wei-Shaw/sub2api v0.1.76](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.76)
>
> 上游 v0.1.53 → v0.1.76，跨 626 commits，808 files changed (+154k/-23k)

### Lite 专属改动

- 🔧 **TOTP 2FA 适配** - 移除 EmailService 依赖，改为仅密码验证方式启用/禁用 TOTP
- 🔧 **移除 Fingerprint/Identity 引用** - `gateway_service.go`、`account_usage_service.go`、`claude_usage_service.go` 清理
- 🔧 **移除 RedeemCode 引用** - `admin_service.go` 清理上游新增的 Redeem 相关代码
- 🔧 **API Key 限额方法增强** - `service/api_key.go` 新增 `HasAnyLimit`、`IsXxxLimitExceeded` 方法
- 🔧 **user_repo.go 全面清理** - 移除 AllowedGroups 同步、加载、过滤函数
- 🔧 **前端清理** - 移除 `useSubscriptionStore`、`isSimpleMode`，删除 `ForgotPasswordView`/`ResetPasswordView`
- 🔧 **前端 TOTP 支持** - `auth.ts` 和 auth store 添加 TOTP 2FA 认证流程

### 合并上游核心功能

#### 认证 & 安全
- ✨ **TOTP 双因素认证** - 完整 2FA 功能，Lite 适配为密码验证方式
- ✨ **Refresh Token 机制** - JWT Token 自动刷新
- ✨ **TLS 指纹模拟** - 新增 TLS 指纹模拟功能
- ✨ **Redis TLS 支持** - 支持 TLS 连接 Redis
- ✨ **CSP Nonce 安全** - 实现 CSP nonce 支持解决内联脚本安全问题
- ✨ **HTTP/2 Cleartext (h2c)** - 支持 h2c 连接配置

#### 管理 & 运维
- ✨ **公告系统** - 管理员/用户公告，Header 铃铛通知
- ✨ **错误透传规则** - 全局错误透传规则配置，支持 `skip_monitoring`
- ✨ **使用清理服务** - 用量日志清理任务与统计过滤
- ✨ **数据导入导出** - 账号和代理配置批量导入导出
- ✨ **CRS 同步预览** - 新增 CRS 同步预览和账号选择功能
- ✨ **账号分组过滤** - 账号列表支持按分组筛选
- ✨ **列设置功能** - 账号管理和订阅管理支持自定义列显示
- ✨ **用户余额/并发历史** - 管理后台查看用户历史记录

#### API Key & 分组
- ✨ **API Key quota/expires_at** - 独立配额和过期时间支持
- ✨ **Group fallback_group** - 分组回退配置
- ✨ **Group model_routing** - 分组级别模型路由规则
- ✨ **Group sort_order** - 拖拽排序分组
- ✨ **Group mcp_xml_inject** - MCP XML 注入开关
- ✨ **用户专属分组倍率** - 支持用户专属分组倍率配置

#### Gateway & Antigravity
- ✨ **Antigravity 全面增强** - 模型映射、限流、调度、Ops 统计
- ✨ **Antigravity 动态 URL 排序** - 最近成功的 URL 优先使用
- ✨ **Antigravity 可配置重试** - 按模型类型配置重试次数，最大重试次数可配
- ✨ **Antigravity scope 级别限流** - 区分 URL 级别和账户配额级别的 429 限流
- ✨ **Antigravity Refresh Token 导入** - 批量 RT 导入创建 OAuth 账号
- ✨ **Antigravity Web Search** - 支持 Web 搜索
- ✨ **Smart Retry** - 智能重试（最大 1 次）+ 粘性会话清理
- ✨ **客户端断连检测** - 流式传输中检测客户端断连，继续读取上游数据用于计费
- ✨ **会话 ID 伪装** - 新增会话 ID 伪装功能
- ✨ **5h 窗口费用控制** - 5 小时窗口费用限制和会话数量限制

#### Gemini
- ✨ **Gemini CLI 粘性会话** - 支持 Gemini CLI 粘性会话与跨账号 thoughtSignature 清理
- ✨ **Gemini 图片计费** - Gemini 原生平台图片计费支持
- ✨ **Gemini 长上下文双倍计费** - 200K 长上下文双倍计费
- ✨ **Gemini 模型映射** - gemini-2.5 系列映射到 gemini-3 系列
- ✨ **Gemini API Key 透传** - API Key 账户跳过模型映射检查，直接透传

#### 计费 & 模型
- ✨ **Anthropic 差异化缓存计费** - 区分 5m/1h 缓存创建 token 的差异化计费
- ✨ **GPT-5.3 Codex 模型支持** - 添加 GPT-5.3 系列映射与价格配置
- ✨ **Claude Opus 4.6 Thinking** - 增强 Claude Thinking 模式支持与 Opus 4.6 动态预算适配
- ✨ **按映射模型计费** - Antigravity 支持按映射模型计费

#### 前端
- ✨ **拖拽分组排序** - 分组管理支持拖拽排序
- ✨ **账号限流状态显示** - 直接展示倒计时
- ✨ **账号表格排序/自动刷新** - 默认排序、持久化、自动刷新
- ✨ **OpenAI OAuth 批量 RT** - 支持批量 RT 输入创建
- ✨ **分页跳转优化** - 优化分页跳转与页大小显示
- ✨ **确认对话框组件** - 使用 ConfirmDialog 替代原生 confirm()
- 🐛 **StatCard 数值溢出修复**
- 🐛 **侧边栏 Logo 闪烁修复**
- ⚡ **前端性能优化** - 移除 Google Fonts、路由预加载优化、Caddy 静态资源缓存

#### 修复
- 🐛 **Anthropic 429 限流** - 使用精确窗口重置时间
- 🐛 **SSE 流式响应** - 修复 usage 数据被覆盖、tool 输入参数转换问题
- 🐛 **OpenAI Codex** - 工具过滤、参数标准化、instructions 处理
- 🐛 **粘性会话** - failover 触发缓存计费豁免、sessionHash 碰撞修复
- 🐛 **thinking 块修复** - 修复 thinking 块被意外修改导致的 400 错误
- 🐛 **软删除增强** - 修复删除钩子调用链
- ⚙️ **Go 升级至 1.25.7** - 修复标准库安全漏洞
- ⚙️ **gosec CI 配置** - 配置 gosec 排除规则

### ⚠️ Lite 不可用功能
- `ops_alert_evaluator_service` - 告警邮件通知（依赖 EmailService）
- `ops_scheduled_report_service` - 定时报告邮件（依赖 EmailService）
- 邀请码注册功能（依赖用户注册系统）
- 密码重置邮件（依赖 EmailService）
- 优惠码功能（已删除）

**Full Changelog**: https://github.com/Kevin-Lab777/sub2api/compare/v0.1.53-lite...v0.1.76-lite

---

## v0.1.53-lite (2026-01-13)

> 基于上游 [Wei-Shaw/sub2api v0.1.53](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.53)

### Lite 专属改动
- 🔧 移除 `scheduler_snapshot_service.go` 中的 `RunModeSimple` 引用
- 🔧 保留 Lite 账号调度逻辑（忽略分组限制）
- ✨ **恢复用户 Profile 页面** - 支持查看个人资料和修改密码
  - 新增 `api/user.ts` - 用户 API 模块
  - 新增 `views/user/ProfileView.vue` - 个人资料页面
  - 新增 `components/user/profile/*` - Profile 相关组件
  - 侧边栏底部添加个人资料入口（与深色模式/收起按钮并列）
- 🔧 **Header 余额显示改为今日使用** - 右上角显示今日使用额度而非余额

### 合并上游 v0.1.51 功能
- ✨ **调度器快照缓存** - 引入 outbox 回放机制
- ✨ **实时流量监控** - Ops 新增实时流量数据层和 API
- ✨ **指标阈值配置** - 运维监控添加指标阈值管理
- 🐛 **客户端 IP 记录修复** - usage_logs 正确记录 IP
- 🐛 **Claude Code 检测修复** - 改进客户端检测和请求信息记录
- 🐛 **自定义错误码** - 支持触发停止调度
- 🐛 **Ops 布局修复** - 错误日志和请求详情模态框优化
- ⚙️ **粘性会话优化** - 提升默认等待时长

### 合并上游 v0.1.52 功能
- ✨ **Ops 自动刷新** - 运维监控添加自动刷新配置
- ✨ **count_tokens 过滤** - Ops 支持过滤 count_tokens 错误
- 🐛 **客户端 IP 修复** - 修复反向代理下 IP 获取错误
- 📝 **OpenCode 配置说明** - 添加配置文件路径提示

### 合并上游 v0.1.53 功能
- ✨ **运维监控全屏模式** - Ops Dashboard 支持全屏显示
- ✨ **模型映射增强** - 对所有请求应用模型映射（包括 Codex CLI）
- ✨ **OpenAI GPT-5 模型** - 同步 GPT-5 系列模型列表
- ✨ **Antigravity/Gemini 优化** - 改进思考块处理
- 🐛 **max_output_tokens 处理** - 完善参数处理逻辑
- 🎨 **滚动条优化** - 优化自动隐藏效果
- 📝 **Gemini OpenCode** - 教程 baseURL 改为 v1beta

### ⚠️ Lite 不可用功能
- `ops_alert_evaluator_service` - 告警邮件通知（依赖 EmailService）
- `ops_scheduled_report_service` - 定时报告邮件（依赖 EmailService）

**Full Changelog**: https://github.com/Kevin-Lab777/sub2api/compare/v0.1.50-lite...v0.1.53-lite

---

## v0.1.50-lite (2026-01-13)

> 基于上游 [Wei-Shaw/sub2api v0.1.50](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.50)

### Lite 专属改动
- 🏷️ 重命名为 "Sub2API Lite"
- 🧹 移除 `isSimpleMode` 遗留代码
- 🔧 GroupSelector 始终显示（无 simple/standard 模式区分）
- 🔧 账号列表始终显示分组列
- 🔧 管理员始终显示新手引导按钮

### 合并上游 v0.1.50 功能
- ✨ **运维监控系统** - 健康评分、告警规则、错误追踪、QPS 指标
- ✨ **流超时处理** - Gateway 添加流超时机制
- 🐛 **浮点数精度修复** - 修复余额扣款时 -0.00 显示问题
- 🐛 **仪表盘修复** - 聚合操作时区边界条件修复
- ⚡ **Redis 限流优化** - 原子化操作 + 故障策略
- 🎨 **前端优化** - Select 组件功能和样式改进

### ⚠️ Lite 不可用功能
- `ops_alert_evaluator_service` - 告警邮件通知（依赖 EmailService）
- `ops_scheduled_report_service` - 定时报告邮件（依赖 EmailService）

**Full Changelog**: https://github.com/Kevin-Lab777/sub2api/compare/v0.1.49-lite...v0.1.50-lite

---

## v0.1.49-lite (2026-01-12)

> 基于上游 [Wei-Shaw/sub2api v0.1.46](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.46)

### Lite 专属改动
- 🔧 版本检测修复 - 支持 `0.1.49 Lite` 和 `0.1.49-lite` 格式解析
- 🐛 计费逻辑修复 - 移除中间件余额检查
- 🐛 上游响应处理修复 - 非 JSON 响应正确转发

**Full Changelog**: https://github.com/Kevin-Lab777/sub2api/compare/v0.1.46-lite...v0.1.49-lite

---

## v0.1.46-lite (2026-01-11)

> 基于上游 [Wei-Shaw/sub2api v0.1.46](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.46)

### 首次上游同步
- 合并上游 v0.1.35 → v0.1.46 的所有更新
- 替换所有 GitHub URL 为 fork 仓库

**Full Changelog**: https://github.com/Kevin-Lab777/sub2api/compare/v0.1.35-lite.1...v0.1.46-lite

---

## v0.1.35-lite.1 (2026-01-10)

> 基于上游 [Wei-Shaw/sub2api v0.1.35](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.35)

### 🎉 初始 Lite 版本

**核心改动：**
- ❌ 移除多用户注册/登录系统（仅保留 Admin）
- ❌ 移除兑换码系统
- ❌ 移除优惠码系统
- ❌ 移除用户属性系统
- ❌ 移除 LinuxDO OAuth
- ✅ 保留订阅系统
- ✅ 标准模式改为 **纯限额控制**（不扣余额）
- ✅ API Key 支持独立限额（日/周/月/总额）

**计费逻辑：**
- API Key 限额 > Group 限额 > 无限制（不检查余额）
- 中间件不再检查用户余额
- Handler 执行限额资格检查

**统计：** 182 files changed, +12,689 / -51,010 lines

---

## 🎯 Lite 版本特性

面向 **个人/小团队** 的精简版本：
- ❌ 移除多用户注册/登录系统（仅保留 Admin）
- ❌ 移除兑换码/优惠码系统
- ❌ 移除用户属性系统
- ✅ 保留订阅系统
- ✅ 标准模式改为 **纯限额控制**（不扣余额）
- ✅ API Key 支持独立限额（日/周/月/总额）

## 📦 安装

```bash
# Docker (推荐)
docker pull kevinleee/sub2api-lite-amd64:latest

# 或从源码构建
git clone -b Light https://github.com/Kevin-Lab777/sub2api.git
cd sub2api && make build
```

## 🔗 相关链接

- **Fork 仓库**: https://github.com/Kevin-Lab777/sub2api
- **Light 分支**: https://github.com/Kevin-Lab777/sub2api/tree/Light
- **上游仓库**: https://github.com/Wei-Shaw/sub2api
