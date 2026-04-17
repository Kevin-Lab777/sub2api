# Account 活跃时间段调度功能 (Active Hours)

> 日期：2026-03-15
> 功能：控制账号仅在每天指定时间段内参与调度

---

## 功能概述

允许管理员为账号配置每日活跃时间窗口（如 8:00-22:00），不在窗口内的账号自动跳过调度。支持跨午夜配置（如 22:00-06:00）。

### 存储格式

利用现有 `extra` JSONB 字段，无需数据库迁移：

```json
{
  "active_hours": [
    { "start": 8, "end": 22 }
  ]
}
```

- `start`/`end`: 0-23 整数，表示小时
- 支持跨午夜：`{"start": 22, "end": 6}` → 22:00-次日06:00
- 数组为空或不存在 = 全天可用（向后兼容）

---

## 改动文件清单

### 后端

#### 1. `backend/internal/service/account.go`

| 改动 | 说明 |
|------|------|
| 新增 `timezone` import | 使用项目配置时区 |
| 新增 `ActiveHourRule` struct | `Start`/`End` int 字段 (0-23) |
| 新增 `GetActiveHours()` 方法 | 从 `Extra["active_hours"]` 解析规则数组 |
| 新增 `isInActiveWindow(now)` 方法 | 判断当前小时是否在任一活跃窗口内，支持跨午夜 |
| 修改 `IsSchedulable()` | 在 `return true` 前加 2 行：检查 active_hours，使用 `timezone.Now()` |

**核心逻辑：**

```go
// IsSchedulable() 末尾新增：
if rules := a.GetActiveHours(); len(rules) > 0 && !a.isInActiveWindow(timezone.Now()) {
    return false
}
return true
```

**跨午夜判断逻辑：**

```go
if r.Start <= r.End {
    // 不跨午夜：start <= hour < end
    if hour >= r.Start && hour < r.End { return true }
} else {
    // 跨午夜：hour >= start 或 hour < end
    if hour >= r.Start || hour < r.End { return true }
}
```

#### 2. `backend/internal/handler/dto/types.go`

- 新增 `ActiveHourRule` DTO struct（`Start`/`End` int）
- `Account` DTO 新增 `ActiveHours []ActiveHourRule` 字段（`json:"active_hours,omitempty"`）

#### 3. `backend/internal/handler/dto/mappers.go`

- 在 `AccountFromServiceShallow()` 末尾新增：从 service 层提取 `active_hours` 到 DTO 顶层字段
- 所有账号类型通用（不限平台/类型）

### 前端

#### 4. `frontend/src/types/index.ts`

- 新增 `ActiveHourRule` interface（`start: number, end: number`）
- `Account` interface 新增 `active_hours?: ActiveHourRule[] | null`

#### 5. `frontend/src/components/account/EditAccountModal.vue`

| 改动 | 说明 |
|------|------|
| 新增响应式变量 | `activeHoursEnabled`, `activeHoursStart`(默认8), `activeHoursEnd`(默认22) |
| 新增模板 UI | 开关 toggle + 两个小时下拉框(0-23)，位于 Quota Control 之前 |
| 新增 `loadActiveHoursSettings()` | 从 `account.active_hours` 加载配置 |
| 新增保存逻辑 | 合并 `active_hours` 到 `extra`，位于所有平台特定处理之后 |

#### 6. `frontend/src/components/account/CreateAccountModal.vue`

| 改动 | 说明 |
|------|------|
| 新增响应式变量 | 同 EditAccountModal |
| 新增模板 UI | 同 EditAccountModal |
| `createAccountAndFinish()` | 在 `doCreateAccount` 前注入 `active_hours` 到 extra |
| Anthropic OAuth 流程 (2处) | 在 extra 构建末尾添加 `active_hours` |

#### 7. `frontend/src/i18n/locales/zh.ts`

```typescript
activeHours: {
  label: '活跃时间段',
  hint: '限制账号仅在每天指定时间段内参与调度',
  start: '开始时间',
  end: '结束时间',
  crossMidnightHint: '支持跨午夜设置，例如 22:00 到 06:00 表示晚上 10 点到次日凌晨 6 点'
}
```

#### 8. `frontend/src/i18n/locales/en.ts`

```typescript
activeHours: {
  label: 'Active Hours',
  hint: 'Only schedule this account during specified daily time window',
  start: 'Start Hour',
  end: 'End Hour',
  crossMidnightHint: 'Cross-midnight is supported, e.g. 22:00 to 06:00 means 10 PM to 6 AM next day'
}
```

---

## 未改动的文件（确认无需修改）

| 文件/模块 | 原因 |
|-----------|------|
| Ent schema | 用现有 `extra` JSON 字段 |
| 数据库迁移 | 无新列 |
| 调度器/Gateway | `IsSchedulable()` 已是唯一过滤入口 |
| Repository 层 | `extra` 字段已有完整读写支持 |
| 降级逻辑 | 现有 `fallback_group` + "no available accounts" 已覆盖 |

---

## 时区处理

使用项目已有的 `timezone.Now()` + config 中的 `timezone` 配置（默认 `Asia/Shanghai`），与 API Key Usage 的时区处理一致。

---

## 验证方式

1. **后端编译**：`go build ./...` ✅ 通过
2. **前端编译**：`pnpm build` — 无新增错误（已有的模块缺失错误与本次改动无关）
3. **功能测试**：
   - 创建/编辑账号设置 `active_hours`（如 8:00-22:00）
   - 在窗口外发请求 → 应返回 "no available accounts"
   - 在窗口内发请求 → 正常调度
   - 跨午夜配置（如 22:00-06:00）→ 验证深夜和凌晨均可调度
