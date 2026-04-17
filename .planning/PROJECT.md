# Sub2API Lite

## What This Is

Sub2API Lite 是基于 `Wei-Shaw/sub2api` 维护的轻量化分支，面向个人和小团队场景提供统一的 AI API 网关、账号调度、配额控制和管理后台。它保留核心网关、分组、账号、API Key、订阅和运维能力，同时移除了多用户自助注册、兑换码、优惠码和用户属性等对 Lite 模式不必要的子系统。

当前仓库既是可运行产品，也是持续同步上游的 fork。除了代码本身，merge playbook、变更清单、发布记录和规划文档同样属于产品维护面的一部分。

## Core Value

在保持上游持续同步能力的前提下，稳定提供精简后的 AI API 网关能力。

## Requirements

### Validated

- ✓ Lite 模式已完成从多用户 SaaS 到 Admin-first 网关的精简改造
- ✓ 标准模式不再扣余额，而是使用 group/API Key 限额模型
- ✓ 管理后台保留账号、分组、API Key、订阅、设置、用量与观测能力
- ✓ 前后端均已适配 Lite 删除项和保留项
- ✓ Lite 分支具备按上游版本持续同步的发布与变更记录

### Active

- [ ] 将历史 `_bmad-output` 规划资料迁移为 `.planning` 下的 GSD canonical docs
- [ ] 让上游同步、Lite 特殊处理规则和发布记录都能被后续 GSD workflow 直接消费
- [ ] 把后续 Lite 维护任务沉淀为可追踪的 phase / quick task，而不是散落在外部文档目录

### Out of Scope

- 终端用户自助注册、邮箱验证、LinuxDO OAuth 等自助认证流程
- 兑换码、优惠码、用户属性及其关联 schema/service/handler
- 恢复原版 standard 模式的余额扣费逻辑
- 依赖 EmailService、TurnstileService、IdentityService 的非核心子系统

## Context

- 仓库是 `Wei-Shaw/sub2api` 的 Lite fork，当前基线为 `v0.1.109-lite`
- 后端使用 Go + Gin + Ent，前端使用 Vue 3 + TypeScript + Vite，数据层为 PostgreSQL + Redis
- 现有规划资料主要保存在仓库外部的 `_bmad-output/planning-artifacts/`，包括技术规格、开发策略、进度追踪、merge playbook、变更清单和 release notes
- Lite 分支的维护重点不只是功能开发，还包括每次 upstream merge 后的差异保留、冲突处理和发布文档更新
- 当前 `.planning` 是从既有 BMAD 文档迁移而来，历史 phase 的细粒度 plan artifacts 并未保留在仓库内

## Constraints

- **Tech stack**: 必须延续现有 Go/Vue/PostgreSQL/Redis 技术栈，避免为规划迁移引入新的运行时依赖
- **Fork model**: 规划必须承认 upstream sync 是常态工作，而不是一次性迁移项目
- **Maintainability**: Lite 特殊逻辑必须保持可审计，便于每次 merge 时快速识别保留、删除和手动合并区域
- **Repository layout**: GSD canonical docs 必须落在仓库内的 `.planning/`，方便后续 agent 和 git commit 一起追踪
- **Language**: 当前维护文档以中文为主，需要兼容现有中文资料与工作流表达

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| 保留 Admin-first 网关与订阅能力，移除多用户自助子系统 | Lite 目标是个人/小团队维护成本最低化 | ✓ Good |
| Standard 模式改为限额控制而非余额扣费 | 保留使用统计能力，但去掉余额体系复杂度 | ✓ Good |
| 继续采用 upstream merge + Lite 规则保留的维护方式 | fork 需要持续吸收上游能力，不能长期脱节 | ✓ Good |
| 将 `_bmad-output` 资料迁移到 `.planning` | 让规划上下文回到仓库内，供 GSD 直接消费 | — Pending |

---
*Last updated: 2026-04-17 after bootstrapping `.planning` from legacy BMAD artifacts*
