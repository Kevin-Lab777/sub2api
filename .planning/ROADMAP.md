# Roadmap: Sub2API Lite

## Overview

Sub2API Lite 的路线已经完成从原版 Sub2API 到精简 fork 的主体改造，当前重点从“删除与修复”转向“持续同步上游并降低维护成本”。历史 1-9 阶段来自已有 Lite 转换记录；接下来的工作重点是把这些维护知识沉淀为仓库内可执行的 GSD 规划体系。

## Phases

**Phase Numbering:**
- 整数 phase 代表规划内里程碑工作
- quick task 代表不进入 roadmap 的临时维护动作

- [x] **Phase 1: Lite 子系统删减** - 删除非 Lite 必需模块并简化保留接口
- [x] **Phase 2: Schema 与迁移调整** - 加入 Lite 限额字段并补数据库迁移
- [x] **Phase 3: Wire 与依赖清理** - 清除已删除服务的依赖注入与 provider
- [x] **Phase 4: Service 依赖修复** - 修复删减后的 service/handler 依赖关系
- [x] **Phase 5: Ent 重新生成** - 重新生成 ent 代码并恢复编译链路
- [x] **Phase 6: 编译与构建修复** - 修复后端/前端删减后的编译与装配问题
- [x] **Phase 7: Lite 核心业务逻辑** - 完成限额、用量和管理侧核心业务
- [x] **Phase 8: 前端适配** - 删除无效页面并完成 Lite 化前端适配
- [x] **Phase 9: 验证与发布同步** - 完成验证、上游同步与版本发布记录
- [ ] **Phase 10: 规划系统迁移** - 将历史 BMAD 文档迁移到 `.planning` 并建立 canonical docs

## Phase Details

### Phase 1: Lite 子系统删减
**Goal**: 移除邮件、兑换码、优惠码、用户属性和非 Lite 用户侧入口，只保留 Lite 核心路径。
**Depends on**: Nothing (historical starting point)
**Requirements**: [ADMIN-01, LITE-03]
**Success Criteria** (what must be TRUE):
  1. 已删除子系统的 handler/service/schema 不再参与构建。
  2. Lite 需要保留的简化文件有明确边界。
  3. 路由和依赖图不再引用已删除模块。
**Plans**: 1 plan

Plans:
- [x] 01-01: 删除或简化非 Lite 必需代码路径

### Phase 2: Schema 与迁移调整
**Goal**: 为 Lite 限额模型补齐 schema 字段和数据库迁移。
**Depends on**: Phase 1
**Requirements**: [LITE-02]
**Success Criteria** (what must be TRUE):
  1. API Key/group 限额字段存在并可持久化。
  2. 已删除 schema 的 edge 不再阻塞 ent 生成。
  3. 数据库迁移可以落地 Lite 所需变更。
**Plans**: 1 plan

Plans:
- [x] 02-01: 完成 schema 改造和 `100_lite_mode.sql` 迁移

### Phase 3: Wire 与依赖清理
**Goal**: 清除删减模块留下的 provider、构造函数和注入残留。
**Depends on**: Phase 2
**Requirements**: [LITE-03]
**Success Criteria** (what must be TRUE):
  1. `wire.go` 和 repository/provider 不再引用已删除服务。
  2. 构建链路不因 provider 缺失而失败。
  3. Lite 依赖图只保留真实运行所需对象。
**Plans**: 1 plan

Plans:
- [x] 03-01: 清理 service/repository/cmd 层注入配置

### Phase 4: Service 依赖修复
**Goal**: 修复删减后 service 层、handler 层和设置路径中的编译与运行依赖。
**Depends on**: Phase 3
**Requirements**: [GATE-02, LITE-03]
**Success Criteria** (what must be TRUE):
  1. 关键 service 不再访问已删除依赖。
  2. Admin 与 gateway 路径维持可运行状态。
  3. 关键错误定义和共用逻辑已迁移到可保留位置。
**Plans**: 1 plan

Plans:
- [x] 04-01: 修复删减后的 service/handler 依赖

### Phase 5: Ent 重新生成
**Goal**: 重新生成 ent 代码并恢复实体层可用状态。
**Depends on**: Phase 4
**Requirements**: [GATE-02]
**Success Criteria** (what must be TRUE):
  1. `go generate ./ent/...` 可以运行。
  2. 临时 stub 与生成代码让构建恢复。
  3. schema 变化正确反映到 ent 层。
**Plans**: 1 plan

Plans:
- [x] 05-01: 重新生成 ent 并修复生成链路

### Phase 6: 编译与构建修复
**Goal**: 清理删减后的后端、路由、CLI 与前端构建错误。
**Depends on**: Phase 5
**Requirements**: [ADMIN-01, ADMIN-02]
**Success Criteria** (what must be TRUE):
  1. `go build ./...` 可以通过。
  2. 前端路由、布局和 API 导入与 Lite 保留集一致。
  3. 已删除入口不会在构建或导航中残留。
**Plans**: 1 plan

Plans:
- [x] 06-01: 修复前后端构建链路与路由引用

### Phase 7: Lite 核心业务逻辑
**Goal**: 完成 Lite 的限额、用量记录和管理侧关键接口逻辑。
**Depends on**: Phase 6
**Requirements**: [GATE-01, LITE-01, LITE-02]
**Success Criteria** (what must be TRUE):
  1. Standard 模式改为限额逻辑而不是余额扣费。
  2. API Key 用量和 reset 操作可以工作。
  3. Gateway 返回的数据包含 Lite 所需的限额信息。
**Plans**: 1 plan

Plans:
- [x] 07-01: 实现 Lite 限额、用量与管理接口

### Phase 8: 前端适配
**Goal**: 将前端页面、路由、类型和设置 UI 调整为 Lite 模式。
**Depends on**: Phase 7
**Requirements**: [ADMIN-01, ADMIN-02]
**Success Criteria** (what must be TRUE):
  1. 不存在的页面和导航项已清除。
  2. Lite 保留能力在前端有可用入口。
  3. 类型和 API 调用与后端 Lite 能力对齐。
**Plans**: 1 plan

Plans:
- [x] 08-01: 完成 Lite 前端页面与导航适配

### Phase 9: 验证与发布同步
**Goal**: 验证 Lite fork 可构建、可运行，并完成到 `v0.1.109-lite` 的上游同步记录。
**Depends on**: Phase 8
**Requirements**: [GATE-03, ADMIN-03]
**Success Criteria** (what must be TRUE):
  1. 前后端构建与运行时验证完成。
  2. release notes、变更清单和进度文档与当前版本一致。
  3. 上游同步引入的新功能和 Lite 特殊修复都有记录。
**Plans**: 1 plan

Plans:
- [x] 09-01: 完成验证并同步到 `v0.1.109-lite`

### Phase 10: 规划系统迁移
**Goal**: 把 `_bmad-output` 里的历史规划资料迁移到仓库内 `.planning`，并建立后续 GSD workflow 的 canonical docs。
**Depends on**: Phase 9
**Requirements**: [SYNC-01, SYNC-02, SYNC-03]
**Success Criteria** (what must be TRUE):
  1. `.planning` 内存在 PROJECT/REQUIREMENTS/ROADMAP/STATE 基线。
  2. 历史 BMAD 资料在 `.planning/docs/` 内有仓库内落点和索引。
  3. 后续 agent 不再依赖仓库外 `_bmad-output` 作为主入口。
**Plans**: 1 plan

Plans:
- [ ] 10-01: 导入 legacy planning docs 并建立 canonical mapping

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Lite 子系统删减 | 1/1 | Complete | 2026-01-11 |
| 2. Schema 与迁移调整 | 1/1 | Complete | 2026-01-11 |
| 3. Wire 与依赖清理 | 1/1 | Complete | 2026-01-11 |
| 4. Service 依赖修复 | 1/1 | Complete | 2026-01-11 |
| 5. Ent 重新生成 | 1/1 | Complete | 2026-01-11 |
| 6. 编译与构建修复 | 1/1 | Complete | 2026-01-11 |
| 7. Lite 核心业务逻辑 | 1/1 | Complete | 2026-01-11 |
| 8. 前端适配 | 1/1 | Complete | 2026-01-11 |
| 9. 验证与发布同步 | 1/1 | Complete | 2026-04-09 |
| 10. 规划系统迁移 | 0/1 | Not started | - |
