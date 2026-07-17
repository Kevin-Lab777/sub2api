.PHONY: build build-backend build-frontend test test-backend test-frontend test-frontend-critical

FRONTEND_CRITICAL_VITEST := \
	src/composables/__tests__/useModelWhitelist.spec.ts \
	src/components/account/__tests__/EditAccountModal.spec.ts \
	src/views/admin/ops/utils/__tests__/errorDetailResponse.spec.ts \
	src/views/admin/ops/components/__tests__/OpsOpenAITokenStatsCard.spec.ts \
	src/components/admin/usage/__tests__/UsageFilters.spec.ts

# 一键编译前后端
build: build-backend build-frontend

# 编译后端（复用 backend/Makefile）
build-backend:
	@$(MAKE) -C backend build

# 编译前端（需要已安装依赖）
build-frontend:
	@pnpm --dir frontend run build

# 运行测试（后端 + 前端）
test: test-backend test-frontend

test-backend:
	@$(MAKE) -C backend test

test-frontend:
	@pnpm --dir frontend run lint:check
	@pnpm --dir frontend run typecheck
	@$(MAKE) test-frontend-critical

test-frontend-critical:
	@pnpm --dir frontend exec vitest run $(FRONTEND_CRITICAL_VITEST)
