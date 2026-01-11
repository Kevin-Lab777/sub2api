-- [LITE] Lite 模式数据库迁移
-- Sub2API Lite v0.1.35-lite.1

-- 1. API Key 限额字段
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS daily_limit_usd DECIMAL(20,8);
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS weekly_limit_usd DECIMAL(20,8);
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS monthly_limit_usd DECIMAL(20,8);
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS total_limit_usd DECIMAL(20,8);

-- 2. API Key 用量追踪字段
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS daily_usage_usd DECIMAL(20,8) NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS weekly_usage_usd DECIMAL(20,8) NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS monthly_usage_usd DECIMAL(20,8) NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS total_usage_usd DECIMAL(20,8) NOT NULL DEFAULT 0;

-- 3. API Key 重置时间字段
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS usage_reset_daily TIMESTAMPTZ;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS usage_reset_weekly TIMESTAMPTZ;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS usage_reset_monthly TIMESTAMPTZ;

-- 4. 创建用量追踪索引
CREATE INDEX IF NOT EXISTS idx_api_keys_usage_reset
ON api_keys(usage_reset_daily, usage_reset_weekly, usage_reset_monthly);

-- 5. 可选：保留删除表的数据以备回滚，不实际删除
-- 如果需要清理可以取消注释以下语句
-- DROP TABLE IF EXISTS redeem_codes CASCADE;
-- DROP TABLE IF EXISTS promo_codes CASCADE;
-- DROP TABLE IF EXISTS promo_code_usages CASCADE;
-- DROP TABLE IF EXISTS user_attribute_definitions CASCADE;
-- DROP TABLE IF EXISTS user_attribute_values CASCADE;
-- DROP TABLE IF EXISTS user_allowed_groups CASCADE;
