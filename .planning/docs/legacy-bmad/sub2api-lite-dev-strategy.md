# Sub2API Lite 开发策略

> 基于 Wei-Shaw/sub2api 的轻量化分支
> 日期: 2026-01-10
> 最后更新: 2026-04-08
> 当前版本: v0.1.109-lite
> Light 分支: https://github.com/Kevin-Lab777/sub2api/tree/Light

---

## 核心策略

```
┌─────────────────────────────────────────────────────────────┐
│  策略: 删除不需要的代码 + 简化保留部分功能                     │
├─────────────────────────────────────────────────────────────┤
│  ✓ 完全删除: 邮件/验证、兑换码、优惠码、用户属性等子系统       │
│  ✓ 简化保留: auth/user/api_key/usage handler (有 [LITE] 标记)│
│  ✓ 正常保留: 账号、代理、分组、API Key、订阅等核心功能        │
│  ✓ API Key 增加日/周/月限额，替代余额计费                     │
│  ✓ 零上下文 Merge 工作流处理上游更新                          │
└─────────────────────────────────────────────────────────────┘
```

---

## 文档索引

| 文档 | 用途 |
|------|------|
| **MERGE_PLAYBOOK.md** | 零上下文 merge 操作手册（喂给 AI 即可处理冲突） |
| **sub2api-lite-changes.md** | 详细变更清单（删除/简化/修改文件分类） |
| sub2api-lite-tech-spec.md | 详细技术规格 |
| sub2api-lite-progress.md | 转换进度跟踪 |
| sub2api-lite-git-workflow.md | Git 工作流记录 |
| api-architecture.md | 原版架构参考 |

---

## Git 工作流

```
上游 (upstream)                    你的 Fork
Wei-Shaw/sub2api                  Kevin-Lab777/sub2api
      │                                  │
      │ fetch                            │
      ▼                                  │
    main ─────────────────────────► Light (开发分支)
```

### 同步上游

```bash
# 1. 获取上游
git fetch upstream

# 2. merge 到 Light 分支
git checkout Light
git merge upstream/main

# 3. 如有冲突，把 MERGE_PLAYBOOK.md 喂给 AI
```

---

## 文件处理分类

| 分类 | 说明 | 示例 |
|------|------|------|
| 🔴 完全删除 | 不需要的子系统 | email, redeem, promo, user_attribute |
| 🟡 简化保留 | 保留核心功能 | auth_handler, user_handler, routes/auth.go |
| 🟠 Lite 修改 | 有 [LITE] 标记 | billing_cache_service, gateway_service |
| 🟢 正常 Merge | 无修改 | account, proxy, group, ops_* |

---

## Lite 模式核心改动

| 改动 | 说明 |
|------|------|
| 移除余额计费 | 标准模式不扣余额，使用限额控制 |
| API Key 限额 | daily/weekly/monthly_limit_usd 字段 |
| 用量追踪 | daily/weekly/monthly_usage_usd 字段 |
| 中间件简化 | 移除余额检查逻辑 |

---

## 版本历史

| 版本 | 基于上游 | 说明 |
|------|----------|------|
| v0.1.35-lite.1 | v0.1.35 | 初始 Lite 版本 |
| v0.1.46-lite | v0.1.46 | 首次上游同步 |
| v0.1.50-lite | v0.1.50 | 同步 Ops 监控系统 |
| v0.1.53-lite | v0.1.53 | 恢复用户 Profile 页面 |
| v0.1.76-lite | v0.1.76 | 同步 TOTP/公告/错误透传等功能 |
| v0.1.83-lite | v0.1.83 | 同步 Cache TTL Override/Codex 修复等 |
| v0.1.88-lite | v0.1.88 | 同步 Sora/IP白名单/quota+rate_limit/幂等性等大规模重构 |
| v0.1.103-lite | v0.1.103 | 同步 DB备份/Backend Mode/端点可观测性/upstream model/分组容量等 |
| v0.1.109-lite | v0.1.109 | 同步 Channel定价/Sora移除/gpt-5.4-mini/虚拟滚动/requested model追踪等 |

