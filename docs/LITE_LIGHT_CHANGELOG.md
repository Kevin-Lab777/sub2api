# Sub2API Light Branch Changelog

This document records downstream-only maintenance on the `Light` branch after
syncing from upstream `Wei-Shaw/sub2api`.

## 2026-07-17 - Upstream Sync to v0.1.159-lite

Base state:

- Branch: `Light`
- Previous downstream head: `cfda0a82`
- Previous upstream baseline: `upstream/main@6f43986c`
- Merge target: `upstream/main@c2c19a7c` (`v0.1.159-1-gc2c19a7cb`)
- Upstream release tag: `v0.1.159` (`2a2b5826`)
- Merge delta: 535 commits, 875 files, 97 conflicts
- Lite version file: `backend/cmd/server/VERSION` = `0.1.159-Lite`

Retained upstream capabilities:

- Admin audit logs and step-up TOTP for sensitive operations.
- Async image tasks, Grok import/quota/video improvements, and upstream billing probes.
- Channel monitor, scheduler snapshot, usage metadata, static cache, deployment,
  and security middleware updates.
- Curated model additions: `gpt-5.6`, `grok-4.5`, `grok-4.5-latest`,
  `grok-build-latest`, and `composer-2.5`.

Lite preservation work:

- Kept registration, redeem, promo, user attributes, user management UI,
  normal-user payment UI, SMTP tests, and full user OAuth flows disabled or
  deleted.
- Kept email and DingTalk OAuth handlers and dependent tests behind the
  `full` build tag.
- Removed the orphaned Full-profile OAuth invitation test that imported the
  deleted RedeemCode schema, restoring `go mod tidy` and GoReleaser hooks.
- Kept standard mode quota-based: non-subscription gateway usage records cost
  and quota without deducting user balance.
- Added AuthService compatibility session claims and no-op session-family
  revocation without restoring the removed identity domain.
- Kept email queue wiring nil and made balance recharge fulfillment return
  `BALANCE_RECHARGE_DISABLED` without restoring `RedeemService`.
- Preserved downstream Active Hours and the account Group Selector behavior.
- Restored repository scoped-key locking for normalized-email concurrency and
  recognized all synthetic OAuth email domains as reserved.

Validation:

- `go generate ./ent` and `go generate ./cmd/server` passed.
- `go build ./...` and `go test ./...` passed.
- `make test-unit` and `make test-integration` passed in `backend`.
- `pnpm build` passed in `frontend`.
- `make test-frontend` passed: 5 files, 62 tests.
- golangci-lint v2.9 passed with `--tests=false`: 0 issues.
- GoReleaser's before hook and five-target snapshot build passed.
- Conflict marker, deleted-module, and Lite balance invariant scans passed.

## 2026-07-10 - OpenAI Model Cleanup and 2FA Readiness Check

Base state:

- Branch: `Light`
- Upstream merge baseline: `upstream/main@6f43986c`
- Lite merge commit: `65b22607`
- Lite version file: `backend/cmd/server/VERSION` = `0.1.146-Lite`

Downstream changes:

- Trimmed the OpenAI account whitelist UI to the Lite curated default model set:
  - `gpt-5.6-sol`
  - `gpt-5.6-terra`
  - `gpt-5.6-luna`
  - `gpt-5.5`
  - `gpt-5.4`
  - `gpt-5.4-mini`
  - `gpt-5.3-codex-spark`
  - `codex-auto-review`
  - `gpt-5.2`
  - `gpt-image-1`
  - `gpt-image-1.5`
  - `gpt-image-2`
- Removed noisy OpenAI preset entries for legacy or non-curated models:
  `gpt-4o`, `gpt-4o-mini`, `gpt-4.1`, `o1`, `o3`, dated `gpt-5.2`
  snapshots, `gpt-5.2-pro`, and audio/realtime preview models.
- Updated OpenAI fallback defaults from `gpt-4o` to `gpt-5.4`.
- Updated OpenAI monitor examples and channel placeholders to `gpt-5.4-mini`.
- Aligned the OpenCode key template with the Lite Codex model by replacing
  `codex-mini-latest` with `codex-auto-review`.
- Confirmed TOTP 2FA remains available in Lite when:
  - `TOTP_ENCRYPTION_KEY` is configured as a fixed 64-character hex key.
  - Admin settings enable `totp_enabled`.
  - Users enable TOTP from Profile with password verification.
- Aligned GitHub Actions with the downstream Light profile:
  - Replaced upstream/full frontend critical tests with Lite-safe tests.
  - Limited backend unit and integration CI targets to packages retained in
    Light.
  - Ran `golangci-lint` without deleted upstream test packages.
  - Bumped CI, release, and Docker Go patch version checks to `1.26.5`.

Validation:

- Frontend targeted tests passed:
  - `src/composables/__tests__/useModelWhitelist.spec.ts`
  - `src/components/account/__tests__/EditAccountModal.spec.ts`
  - `src/views/admin/ops/utils/__tests__/errorDetailResponse.spec.ts`
  - `src/views/admin/ops/components/__tests__/OpsOpenAITokenStatsCard.spec.ts`
  - `src/components/admin/usage/__tests__/UsageFilters.spec.ts`
- `make test-frontend` passed.
- `make build` passed in `backend`.
- `make test-unit` passed in `backend`.
- `make test-integration` passed in `backend`.
- `golangci-lint run --timeout=30m --tests=false` passed in `backend`.
- `govulncheck ./...` passed in `backend` with 0 reachable vulnerabilities.
