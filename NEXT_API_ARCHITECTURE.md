# Next API Architecture

Next API is the administrator-facing account gateway used by New API. This
repository starts from upstream Sub2API so that provider account support,
OAuth refresh, scheduling, and protocol forwarding can continue to track the
original project without carrying its customer SaaS product into the request
path.

## Ownership

| Concern | New API | Next API |
| --- | --- | --- |
| End users and customer authentication | Owner | None |
| Customer API keys | Owner | None |
| Payments, balances, subscriptions, and commercial quotas | Owner | None |
| Commercial groups and model permissions | Owner | None |
| Customer RPM and concurrency | Owner | None |
| Cross-channel selection and retry | Owner | None |
| Technical account-pool groups | References pool ID | Owner |
| Provider account authorization and OAuth refresh | None | Owner |
| Account scheduling, concurrency, RPM, and cooldowns | None | Owner |
| Proxies and provider protocol forwarding | None | Owner |
| Sticky upstream sessions and account-pool failover | Supplies session ID | Owner |
| Customer billing and authoritative usage records | Owner | Reports raw usage |
| Account health and upstream usage telemetry | Consumes as needed | Owner |

New API selects a commercial route and a Next API pool. Next API may fail over
only among accounts inside that pool. It must not select another commercial
channel, charge a user, or apply a second customer quota.

## Invocation Boundary

New API imports `gatewaycore.Runtime` and passes its existing
`http.ResponseWriter` and `*http.Request` directly. This preserves SSE,
WebSocket upgrades, multipart uploads, cancellation, and backpressure without
loopback HTTP, Docker DNS, or response buffering.

Every invocation provides:

- a positive technical `PoolID`;
- New API's non-empty request and session identifiers;
- an explicit client protocol and requested model;
- a required raw-usage callback.

The usage callback returns token and media measurements to New API. Next API
does not calculate customer prices or mutate customer balances.

`gatewaycore.Engine` now implements this boundary. It resolves the exact
technical pool selected by New API, rejects inactive or inconsistent pools,
dispatches to the explicitly registered Anthropic, OpenAI, or Gemini protocol
implementation, validates raw upstream measurements, and invokes the caller's
usage callback. It has no default protocol, cross-pool fallback, customer
principal, or billing path.

The technical scheduler accepts only pool identity, model, session identity,
and excluded account IDs. It does not accept New API user IDs or provider
metadata user IDs. A pool-level client restriction rejects the invocation in
place; configured legacy fallback-group IDs are never traversed by scheduling.

The Anthropic provider path now has a framework-independent transport exchange
implemented by both the transitional Gin handler and the native `net/http`
runtime. Direct Anthropic, API-key passthrough, Bedrock, streaming, and local
web-search responses use the same provider code without constructing a Gin
context. The native Anthropic dispatcher parses the original request, enforces
invocation/body model consistency, schedules and fails over only inside the
resolved pool, acquires account concurrency leases, and returns raw
measurements.

OpenAI and Gemini still need native dispatchers before a complete production
`gatewaycore.Runtime` can be assembled. New API does not import this partial
runtime yet. The existing customer-authenticated HTTP handlers therefore remain
transitional code rather than the final in-process path.

## Removal Sequence

1. Introduce the public runtime contract and a dedicated dependency graph.
2. Move gateway handlers onto a technical pool principal and remove customer
   API-key authentication, user concurrency, billing eligibility, and user RPM
   checks from the hot path.
3. Integrate New API with the in-process runtime, keeping its current customer
   billing, routing, retry, and rate-limit behavior authoritative.
4. Remove public user, payment, redeem, promotion, affiliate, subscription, and
   customer API-key routes, handlers, services, workers, repositories, schema,
   and frontend pages from Next API.
5. Retain only administrator authentication plus account-pool, account, OAuth,
   proxy, scheduling, operations, and account-level usage management.

## Current Progress

- The public runtime contract and protocol-independent engine are implemented
  and tested.
- Pool resolution loads the authoritative technical group once and binds it to
  the invocation context for scheduler reuse.
- Account scheduling no longer accepts customer identity arguments and cannot
  switch from the requested pool to a fallback group.
- Administrator password/TOTP authentication and refresh-token sessions use a
  dedicated `AdminAuthService` dependency graph. It does not construct promo,
  redeem, affiliate, subscription, or customer OAuth services.
- Unregistered customer registration, social OAuth, affiliate, platform-quota,
  identity-binding, and notification-email handlers have been physically
  removed with their route-specific tests.
- Customer self-service and commerce routes are no longer registered.
- Inactive customer, payment, subscription, announcement, affiliate, promo,
  redeem, user-attribute, risk-control, model-plaza, and payment-route handlers
  are no longer members of the Wire handler aggregation graph.
- The administrator frontend exposes only the retained gateway management
  routes.
- Customer self-service, registration and social OAuth, payment consumption,
  subscriptions, affiliate, redeem, promotion, announcement, risk-control,
  model-plaza, customer-management, and payment-provider frontend
  implementations have been physically removed. Stripe and Airwallex browser
  SDKs are no longer runtime dependencies.
- Customer-owned asynchronous image-task and batch-image HTTP routes are no
  longer registered, and their queues, cleanup services, workers, and the
  otherwise-idle payment-order expiry worker are absent from the application
  dependency graph. Synchronous provider image forwarding remains available.
- The administrator settings surface now contains only gateway cooldowns,
  panel rate limits, stream timeout handling, request rectification, Anthropic
  beta policy, web-search emulation, administrator API-key management, and
  backups. The former SaaS-wide settings and email-template routes, handlers,
  DTOs, payment-provider editors, and asynchronous image-storage settings have
  been physically removed.
- Ops console discovery uses the dedicated `/admin/ops/capabilities` contract.
  It no longer loads the removed SaaS settings or payment configuration, and
  it does not persist server capability flags in browser storage.
- Payment providers, image-storage providers, customer notification services,
  Turnstile, user attributes, and customer TOTP/user services are absent from
  the administrator settings dependency graph.
- Provider forwarding now uses a real HTTP exchange with typed request state,
  response status/byte tracking, and streaming flush support. Existing Gin
  routes are thin adapters over that exchange.
- A native Anthropic dispatcher enforces exact-pool scheduling, strict account
  wait-queue errors, no failover after response commitment, sticky-session
  binding, account RPM updates, and raw token/cache/web-search measurements.
- The legacy gateway handlers, customer authentication implementation,
  customer caches/workers, and customer Ent schemas still require separation
  or deletion. OpenAI and Gemini provider services also still depend on Gin and
  block complete runtime assembly. Their presence is tracked as unfinished
  work, not as a runtime compatibility mechanism.

Each removal is complete only when its route, dependency injection provider,
background worker, persistence schema, generated ORM code, frontend entry, and
tests are all gone. A disabled route with live workers is not considered
removed.

## Light Reference Policy

The existing `sub2api` Light branch is a behavioral reference, not a source
tree to copy. Reuse is limited to reviewed changes that still match current
upstream behavior, especially administrator-only authentication, group UI
retention, and prior SaaS deletion inventories. Upstream `main` remains the
merge base for Next API.
