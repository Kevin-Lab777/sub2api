# Sub2API Lite Git 工作流记录

**日期**: 2026-01-11
**状态**: 已完成

---

## 仓库配置

### Remote 配置

```bash
# 原始配置
origin    https://github.com/Wei-Shaw/sub2api.git

# 重新配置后
origin    https://github.com/Kevin-Lab777/sub2api.git (你的 fork)
upstream  https://github.com/Wei-Shaw/sub2api.git (上游仓库)
```

### 配置命令

```bash
# 将原 origin 改名为 upstream
git remote rename origin upstream

# 添加你的 fork 作为新的 origin
git remote add origin https://github.com/Kevin-Lab777/sub2api.git

# 验证配置
git remote -v
```

---

## 分支策略

| 分支 | 用途 | 来源 |
|------|------|------|
| `main` | 跟踪上游主分支 | upstream/main |
| `Light` | Lite 版本开发分支 | 从 main 创建 |

---

## 完整操作流程

### 1. 配置 Git Remote

```bash
cd /Users/kevinlee/Documents/Work/AIRelayServices/sub2api

# 重命名原 origin 为 upstream
git remote rename origin upstream

# 添加 fork 仓库为 origin
git remote add origin https://github.com/Kevin-Lab777/sub2api.git

# 验证
git remote -v
# origin    https://github.com/Kevin-Lab777/sub2api.git (fetch)
# origin    https://github.com/Kevin-Lab777/sub2api.git (push)
# upstream  https://github.com/Wei-Shaw/sub2api.git (fetch)
# upstream  https://github.com/Wei-Shaw/sub2api.git (push)
```

### 2. 创建 Light 分支

```bash
# 确保在 main 分支
git checkout main

# 创建并切换到 Light 分支
git checkout -b Light
```

### 3. 提交 Lite 修改

```bash
# 添加所有修改
git add -A

# 提交 Lite 核心改动
git commit -m "feat: Sub2API Lite mode - lightweight version for personal/small team use

Major changes:
- Remove multi-user registration/login system (admin only)
- Remove redeem code system
- Remove promo code system
- Remove user attribute system
- Remove LinuxDO OAuth integration
- Keep subscription system
- Standard mode: pure quota control instead of balance deduction
- API Key enhancement: support independent quota limits

Billing logic:
- API Key limits > Group limits > Unlimited (no balance check)
- Middleware no longer checks user balance
- Handler performs quota eligibility check

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

**提交统计**: 182 files changed, 12689 insertions(+), 51010 deletions(-)

### 4. 打版本标签

```bash
git tag -a v0.1.46-lite -m "Sub2API Lite v0.1.46 - Lightweight version for personal/small team use"
```

### 5. 替换 GitHub URL

```bash
# 批量替换所有文件中的 URL
find . -type f \( -name "*.md" -o -name "*.yml" -o -name "*.yaml" -o -name "*.vue" -o -name "*.go" -o -name "*.sh" -o -name "*.service" -o -name "*.ts" \) -not -path "./.git/*" -exec sed -i '' 's|Wei-Shaw/sub2api|Kevin-Lab777/sub2api|g' {} \;

# 验证替换成功
grep -r "Wei-Shaw/sub2api" --include="*.md" . 2>/dev/null | grep -v ".git"
# 应该没有输出

# 提交 URL 更改
git add -A
git commit -m "chore: update GitHub URLs to fork repository

Change all references from Wei-Shaw/sub2api to Kevin-Lab777/sub2api

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

**提交统计**: 246 files changed, 675 insertions(+), 675 deletions(-)

### 6. 推送到远程仓库

```bash
# 推送 Light 分支
git push -u origin Light

# 推送标签
git push origin v0.1.46-lite
```

---

## 最终状态

### 分支

```
* Light    5b2b6c3 chore: update GitHub URLs to fork repository
  main     ff08758 (upstream/main, upstream/HEAD) ...
```

### 标签

```
v0.1.46-lite    Sub2API Lite v0.1.46
```

### 提交历史 (Light 分支)

```
5b2b6c3 chore: update GitHub URLs to fork repository
7c56e05 feat: Sub2API Lite mode - lightweight version for personal/small team use
ff08758 (upstream/main) ... (上游最新提交)
```

---

## 后续同步上游更新

当上游有更新时，按以下步骤同步：

```bash
# 1. 获取上游更新
git fetch upstream

# 2. 切换到 main 分支
git checkout main

# 3. 合并上游更新
git merge upstream/main

# 4. 推送更新后的 main
git push origin main

# 5. 切换到 Light 分支
git checkout Light

# 6. 合并 main 的更新（可能需要解决冲突）
git merge main

# 7. 解决冲突后推送
git push origin Light
```

### 冲突解决参考

合并时可能在以下文件产生冲突：
- `billing_cache_service.go` - Lite 计费逻辑
- `api_key_auth.go` / `api_key_auth_google.go` - 中间件
- `gateway_service.go` - 网关服务

参考 `MERGE_PLAYBOOK.md` 处理冲突。

---

## 相关链接

- Fork 仓库: https://github.com/Kevin-Lab777/sub2api
- 上游仓库: https://github.com/Wei-Shaw/sub2api
- Light 分支: https://github.com/Kevin-Lab777/sub2api/tree/Light
- 创建 PR: https://github.com/Kevin-Lab777/sub2api/pull/new/Light

---

## 文件说明

| 文件 | 说明 |
|------|------|
| `sub2api-lite-tech-spec.md` | Lite 版本详细技术规格 |
| `sub2api-lite-progress.md` | 转换进度跟踪 |
| `sub2api-lite-changes.md` | 变更清单 |
| `MERGE_PLAYBOOK.md` | 上游合并操作手册 |
| `sub2api-lite-git-workflow.md` | 本文档 - Git 工作流 |
