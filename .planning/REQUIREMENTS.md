# Requirements: Sub2API Lite

**Defined:** 2026-04-17
**Core Value:** 在保持上游持续同步能力的前提下，稳定提供精简后的 AI API 网关能力。

## v1 Requirements

### Gateway Core

- [x] **GATE-01**: 管理员可以通过统一 API 访问和转发上游 AI 服务能力
- [x] **GATE-02**: 系统可以管理多账号、分组、代理和调度规则
- [x] **GATE-03**: 系统可以记录请求用量、成本、请求模型和实际上游模型

### Admin Operations

- [x] **ADMIN-01**: 管理后台可以管理账号、分组、API Key、订阅和系统设置
- [x] **ADMIN-02**: 前端保留 Lite 模式必需的仪表盘、账号、用量和设置页面
- [x] **ADMIN-03**: 部署方式继续支持脚本安装和 Docker Compose

### Lite Billing

- [x] **LITE-01**: Lite 标准模式不扣余额，而是允许无限制或按限额控制
- [x] **LITE-02**: API Key 和 Group 支持日/周/月/总额限额以及用量重置
- [x] **LITE-03**: 已删除的子系统不会在后端路由、前端导航或 schema 中重新出现

### Sync and Maintainability

- [ ] **SYNC-01**: 仓库内存在 GSD 可直接消费的 `.planning` canonical docs
- [ ] **SYNC-02**: 上游同步规则、Lite 差异清单和 release notes 都有仓库内入口
- [ ] **SYNC-03**: 历史 BMAD 文档与当前 canonical docs 之间的映射关系清晰可查

## v2 Requirements

### Automation

- **AUTO-01**: 自动从 upstream diff 生成 Lite merge 分类报告
- **AUTO-02**: 自动从 release/tag 信息更新 release notes 和 sync planning docs
- **AUTO-03**: 自动检查 Lite 删除项是否被上游重新引入

## Out of Scope

| Feature | Reason |
|---------|--------|
| 用户自注册、邮箱验证、找回密码 | Lite 模式以 Admin-first 维护为主，用户自助链路已删除 |
| 兑换码、优惠码、用户属性系统 | 不是 Lite 核心能力，且显著增加 merge 复杂度 |
| 恢复余额扣费标准模式 | 与 Lite “简单、低维护”的核心目标冲突 |
| 依赖 EmailService 的告警/报告链路 | 相关依赖已从 Lite 中剔除 |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| GATE-01 | Phase 7 | Complete |
| GATE-02 | Phase 1 | Complete |
| GATE-03 | Phase 9 | Complete |
| ADMIN-01 | Phase 8 | Complete |
| ADMIN-02 | Phase 8 | Complete |
| ADMIN-03 | Phase 9 | Complete |
| LITE-01 | Phase 7 | Complete |
| LITE-02 | Phase 2 | Complete |
| LITE-03 | Phase 1 | Complete |
| SYNC-01 | Phase 10 | Pending |
| SYNC-02 | Phase 10 | Pending |
| SYNC-03 | Phase 10 | Pending |

**Coverage:**
- v1 requirements: 12 total
- Mapped to phases: 12
- Unmapped: 0

---
*Requirements defined: 2026-04-17*
*Last updated: 2026-04-17 after migrating legacy planning intent into GSD structure*
