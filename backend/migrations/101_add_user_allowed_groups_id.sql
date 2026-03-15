-- [LITE] 101: Add auto-increment id column to user_allowed_groups
-- Reason: ent code generator requires an 'id' column for all entities.
-- The table was originally created with composite PK (user_id, group_id).
-- Add a surrogate 'id' column while keeping the unique constraint.

-- Step 1: Drop the composite primary key
ALTER TABLE user_allowed_groups DROP CONSTRAINT IF EXISTS user_allowed_groups_pkey;

-- Step 2: Add auto-increment id column as new primary key
ALTER TABLE user_allowed_groups ADD COLUMN IF NOT EXISTS id BIGSERIAL PRIMARY KEY;

-- Step 3: Re-add unique constraint on (user_id, group_id)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'user_allowed_groups_user_group_unique') THEN
        ALTER TABLE user_allowed_groups ADD CONSTRAINT user_allowed_groups_user_group_unique UNIQUE (user_id, group_id);
    END IF;
END $$;
