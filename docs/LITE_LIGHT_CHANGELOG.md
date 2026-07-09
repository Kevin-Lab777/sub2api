# Sub2API Light Branch Changelog

This document records downstream-only maintenance on the `Light` branch after
syncing from upstream `Wei-Shaw/sub2api`.

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

Validation:

- Frontend targeted tests passed:
  - `src/composables/__tests__/useModelWhitelist.spec.ts`
  - `src/components/account/__tests__/EditAccountModal.spec.ts`
  - `src/views/admin/ops/utils/__tests__/errorDetailResponse.spec.ts`
  - `src/views/admin/ops/components/__tests__/OpsOpenAITokenStatsCard.spec.ts`
  - `src/components/admin/usage/__tests__/UsageFilters.spec.ts`
- `pnpm build` passed.
- `go build ./...` passed.

Known residual test gap:

- `go test ./internal/service ./internal/server` is blocked by pre-existing
  service test compile failures unrelated to this change, including old
  references to `fingerprint`, deleted email queue symbols, and scheduled report
  helpers.
