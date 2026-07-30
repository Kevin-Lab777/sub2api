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
- New API's request and session identifiers;
- an explicit client protocol and requested model;
- a required raw-usage callback.

The usage callback returns token and media measurements to New API. Next API
does not calculate customer prices or mutate customer balances.

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
