# Planning Docs Index

本文件定义 `.planning` 下哪些文档是 **canonical baseline / canonical maintenance inputs**，哪些只是 **reference-only imports**。这次 quick task 只把 legacy BMAD 资料迁入仓库，并没有重新 bootstrap 或取代核心 `.planning` baseline。

## Canonical Baseline

后续 GSD workflow 默认应先读这四个文件：

| File | Role |
|------|------|
| `.planning/PROJECT.md` | 当前项目定位、核心价值、约束、关键决策 |
| `.planning/REQUIREMENTS.md` | 当前 requirement 集、traceability 和 out-of-scope 边界 |
| `.planning/ROADMAP.md` | 历史/当前 phase 结构与未来规划入口 |
| `.planning/STATE.md` | 当前焦点、recent context、quick task 记录 |

这些文件描述 **当前状态**。如果与导入的 legacy 文档冲突，以这里为准。

## Canonical Maintenance

以下导入文档虽然来自 legacy BMAD，但在 Lite fork 的日常维护中仍应被视为 canonical maintenance inputs：

| File | Why it stays canonical for maintenance |
|------|----------------------------------------|
| `.planning/docs/legacy-bmad/MERGE_PLAYBOOK.md` | 上游 merge 冲突处理的主手册 |
| `.planning/docs/legacy-bmad/sub2api-lite-changes.md` | Lite 差异分类与保留策略的主清单 |
| `.planning/docs/legacy-bmad/release-notes.md` | 上游同步与 Lite 专属修复的版本记录 |

## Reference-Only Imports

以下文档保留在仓库内，便于回溯和补充上下文，但不应直接覆盖 canonical baseline：

| File | Use |
|------|-----|
| `.planning/docs/legacy-bmad/sub2api-lite-tech-spec.md` | 回看 Lite 行为、字段和限制设计细节 |
| `.planning/docs/legacy-bmad/sub2api-lite-dev-strategy.md` | 理解 Lite fork 的删减/简化策略 |
| `.planning/docs/legacy-bmad/sub2api-lite-progress.md` | 回填历史 phase 完成记录 |
| `.planning/docs/legacy-bmad/api-architecture.md` | 回忆原始系统边界、模块结构和技术栈 |
| `.planning/docs/legacy-bmad/active-hours-changelog.md` | 追踪 `active_hours` 单功能实现细节 |
| `.planning/docs/legacy-bmad/sub2api-lite-git-workflow.md` | 补充 origin/upstream/Light 工作流细节 |
| `.planning/docs/legacy-bmad/LITE_CHANGES_TEMPLATE.md` | 下一次同步时复用的文档模板 |

## Source Mapping

| Repo path | Legacy source |
|-----------|---------------|
| `.planning/docs/legacy-bmad/sub2api-lite-tech-spec.md` | `../_bmad-output/planning-artifacts/sub2api-lite-tech-spec.md` |
| `.planning/docs/legacy-bmad/sub2api-lite-dev-strategy.md` | `../_bmad-output/planning-artifacts/sub2api-lite-dev-strategy.md` |
| `.planning/docs/legacy-bmad/sub2api-lite-progress.md` | `../_bmad-output/planning-artifacts/sub2api-lite-progress.md` |
| `.planning/docs/legacy-bmad/sub2api-lite-changes.md` | `../_bmad-output/planning-artifacts/sub2api-lite-changes.md` |
| `.planning/docs/legacy-bmad/MERGE_PLAYBOOK.md` | `../_bmad-output/planning-artifacts/MERGE_PLAYBOOK.md` |
| `.planning/docs/legacy-bmad/release-notes.md` | `../_bmad-output/planning-artifacts/release-notes.md` |
| `.planning/docs/legacy-bmad/api-architecture.md` | `../_bmad-output/planning-artifacts/api-architecture.md` |
| `.planning/docs/legacy-bmad/active-hours-changelog.md` | `../_bmad-output/planning-artifacts/active-hours-changelog.md` |
| `.planning/docs/legacy-bmad/sub2api-lite-git-workflow.md` | `../_bmad-output/planning-artifacts/sub2api-lite-git-workflow.md` |
| `.planning/docs/legacy-bmad/LITE_CHANGES_TEMPLATE.md` | `../_bmad-output/planning-artifacts/LITE_CHANGES_TEMPLATE.md` |

## Operating Rules

- 当前状态、当前范围、当前优先级，更新 `.planning/PROJECT.md`、`.planning/REQUIREMENTS.md`、`.planning/ROADMAP.md`、`.planning/STATE.md`。
- merge 冲突处理、Lite 差异分类、release 历史，优先更新 canonical maintenance docs。
- 历史说明、功能专项 changelog、模板和旧版设计细节，更新 `legacy-bmad/` 下对应文档。
- 如果 legacy 文档里的结论已经成为当前事实，应把结论吸收到 canonical baseline，而不是长期让 baseline 依赖 legacy 文档。
