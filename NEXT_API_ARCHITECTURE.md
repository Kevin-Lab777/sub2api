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

The usage callback returns token and media measurements to New API, including
the exact image-token subsets and observed output sizes needed for multimodal
pricing. Next API does not calculate customer prices or mutate customer
balances.

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

The Anthropic and native Gemini provider paths now share a
framework-independent transport exchange implemented by both the transitional
Gin handlers and the native `net/http` runtime. Direct Anthropic, Anthropic
API-key passthrough, Bedrock, and native Gemini streaming/non-streaming
responses use the same provider code without constructing a Gin context. Their
dispatchers parse the original request, enforce invocation model consistency,
schedule and fail over only inside the resolved pool, acquire account
concurrency leases, and return raw measurements.

The native Gemini path forwards the actual provider response. It does not
estimate `countTokens` after an upstream failure, invent thought signatures,
strip tool/thinking history after a validation error, or rewrite empty parts.
Those failures remain visible to the caller. Provider-account RPM limits now
apply to Gemini accounts as well as Anthropic accounts.

OpenAI migration is split by endpoint contract. `/v1/responses` now has a
dedicated native component, but it is deliberately not registered as the
complete OpenAI protocol dispatcher. The current support matrix is:

| OpenAI endpoint family | Native runtime state |
| --- | --- |
| `POST /v1/responses` over HTTP/SSE | Implemented and tested |
| `POST /v1/responses/compact` unary JSON | Implemented and tested |
| `GET /v1/responses` inbound WebSocket v2 | Implemented and tested |
| `POST /v1/chat/completions` direct API-key upstream | Implemented and tested |
| Subscription-account Chat Completions adapter | Implemented and tested |
| Legacy `POST /v1/completions` direct API-key upstream | Implemented and tested |
| Embeddings | Implemented and tested |
| Direct API-key Images generations/edits | Implemented and tested |
| Subscription-account Images adapter | Implemented and tested |
| Frameless Live create and sideband | Implemented and tested |

The native Responses path performs only protocol validation, exact model
mapping, provider authentication, HTTP/SSE transport, terminal SSE collection
for non-streaming clients, and endpoint-schema usage extraction. It rejects
invalid JSON instead of normalizing control bytes, requires exact response
media types, and does not use legacy token aliases or content-hash output
deduplication. It does not inject prompts, drop rejected fields, repair
continuation state, reconstruct missing output, or invent usage. Missing
terminal usage is an error rather than a zero-token success.

The compact path requires an account with explicit compact capability, applies
only exact regular and compact-specific model mappings, and requires the unary
JSON upstream contract. It does not strip `stream`, `store`, cache keys, or any
other request fields; unsupported payloads remain upstream errors instead of
being converted into a synthetic SSE bridge.

The native Responses WebSocket path requires an account-level direct v2
capability and establishes the upstream socket before accepting the downstream
upgrade. Dial and handshake failures may select another account only inside the
same technical pool; after downstream acceptance the account and concurrency
lease remain fixed for the connection. The relay validates the first
`response.create` and every explicitly repeated model, applies only the exact
account model mapping, and otherwise forwards text and binary frames without an
HTTP bridge, connection reuse, payload replay, continuation repair, synthetic
events, or client identity impersonation. Completed-turn usage is parsed from
the exact Responses schema and accumulated into one Engine-validated connection
measurement when the socket closes.

The native direct Chat Completions component currently selects only API-key
accounts with explicit Chat Completions eligibility. It preserves the request
and response protocol, changes only the exact account model mapping, and asks a
streaming upstream for the documented terminal usage chunk required by the
runtime contract. JSON and SSE media types, request model/stream fields,
terminal `[DONE]`, and endpoint-schema usage are validated exactly. It does not
run the legacy client-restriction, fast-policy, silent-refusal, image-bridge,
response-repair, or protocol-probing paths.

Subscription accounts use a distinct strict Chat-to-Responses adapter. It
accepts only request fields, message roles/content, and function-tool shapes
that the conversion can represent; unsupported or ambiguous fields fail before
scheduling side effects instead of being dropped or rewritten. Requests with
valid Chat fields that are not representable by the adapter are routed only to
direct API-key accounts; they are never partially converted for a subscription
account. The upstream always uses the real Responses stream. Non-streaming Chat
responses use the upstream response ID and `created_at`; streaming Chat chunks
begin only after a valid `response.created` and finish only from a real terminal
event with exact usage. The adapter never generates fallback IDs/timestamps,
repairs missing output, finalizes a truncated stream, retries semantic
`response.failed` events, or exposes encrypted reasoning that Chat Completions
cannot carry.

The legacy Completions endpoint shares the same strict direct transport and
account boundary while preserving its prompt-shaped request and
`text_completion` response. It has a distinct account capability and is never
routed to subscription accounts or translated through Chat Completions.

The native Embeddings endpoint selects only exact-pool API-key accounts with
explicit embedding capability. It preserves the request and response schema,
changes only the exact account model mapping, requires a JSON success response,
and reports authoritative prompt, cache, and image-input token counts. Missing,
negative, contradictory, or non-integral usage is a transport failure; it is
never replaced by `total_tokens`, estimated, or accepted as zero usage. Because
Embeddings is stateless, its dispatcher neither reads nor writes sticky-session
bindings.

Direct API-key Images generation and editing preserve the public Image API for
JSON and multipart requests and change only the exact account model mapping.
Non-streaming responses report the actual `data` length. Streaming accepts only
the documented `image_generation.*` or `image_edit.*` SSE families and sums the
exact usage carried by each real completed-image event. It does not infer image
count from request `n`, treat partial images as billable output, deduplicate by
content, guess JSON from an SSE media type, emit keepalives, or synthesize a
completion for a truncated stream. Like Embeddings, Images is stateless and
does not create sticky-session cache entries.

Subscription-account Images uses a separate strict Images-to-Responses
adapter. It preserves JSON generation requests and JSON/multipart edits only
when every field has an exact Responses image-tool representation. Requests
that require a fabricated remote URL, unknown fields, or per-image usage split
from an aggregate multi-image stream are scheduled only to direct API-key
accounts. The adapter requires authoritative `response.created` metadata before
emitting partial frames and a real `response.completed` output, passes through
exact image-tool usage, and reports edit image-input tokens only when the
upstream provides them. It does not use `response.output_item.done` as a
missing-terminal fallback, synthesize
timestamps or completion events, deduplicate image content, finalize truncated
streams, download client assets, or create sticky-session cache entries.

Frameless Live preserves both `POST /v1/live` and
`POST /backend-api/codex/realtime/calls`, plus their call-specific WebSocket
sideband routes. The resolved technical pool carries the administrator's Live
switch, and only exact-model OAuth accounts with explicit Live eligibility may
be scheduled. A short request account slot transitions into an account-only
Redis Live lease; no user or API-key ID is fabricated. The persisted call
mapping contains technical pool, request, and session identity so a sideband
connection cannot cross pools or New API sessions. The observer keeps the
provider-account lease alive while no client controls the sideband and releases
it exactly once when the upstream session ends.

Live still requires a real ChatGPT DeviceCheck attestation. The retained
provider currently obtains that proof from the official ChatGPT.app runtime on
Apple Silicon macOS. Missing platform support, app resources, or attestation
keys remain explicit availability errors; Next API does not bypass or emulate
the proof for Linux containers.

Antigravity accounts participating in Gemini mixed scheduling also retain a
separate Gin-bound forwarder and are not silently routed through the native
Gemini implementation. These paths must be migrated before a complete
production `gatewaycore.Runtime` can be assembled. New API does not import this
partial runtime yet. The existing customer-authenticated HTTP handlers
therefore remain transitional code rather than the final in-process path.

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
- Native Gemini forwarding uses the same transport boundary for Gemini API-key,
  OAuth, and service-account requests. Its dispatcher enforces an exact native
  Gemini pool, strict model/path matching, account concurrency, sticky sessions,
  account RPM, committed-response failover boundaries, and raw token/cache/image
  measurements.
- Synthetic Gemini token estimates and signature/content rectification were
  removed from the native and Anthropic-compat Gemini paths.
- Native OpenAI `POST /v1/responses`, `POST /v1/responses/compact`, and
  `GET /v1/responses` WebSocket v2 forwarding now have strict JSON/model
  validation, exact-pool scheduling, account concurrency leases, API-key and
  subscription-account authentication, raw JSON/SSE forwarding, terminal SSE
  collection, raw WebSocket frame relay, committed-response failover
  boundaries, and raw token/cache/image measurements. WebSocket connections
  aggregate exact completed-turn telemetry into one runtime measurement. The
  dispatcher remains endpoint-specific and is not a complete OpenAI dispatcher.
- Direct API-key `POST /v1/chat/completions` now has a separate strict
  exact-pool dispatcher with raw JSON/SSE forwarding, documented stream-usage
  negotiation, committed-response failover boundaries, and exact
  prompt/completion/cache telemetry. Subscription-account conversion and the
  legacy Completions endpoint uses the same direct transport with a distinct
  capability and raw prompt/text-completion protocol.
- Subscription-account Chat Completions now uses a separately validated
  Responses adapter with deterministic request/output conversion, authoritative
  upstream IDs/timestamps, exact terminal usage, and no semantic-error
  failover. Lossy request shapes are rejected explicitly.
- Native `POST /v1/embeddings` now uses exact API-key capability scheduling,
  raw JSON forwarding, exact model mapping, strict endpoint usage extraction,
  and multimodal image-input token telemetry.
- Direct API-key `POST /v1/images/generations` and `/v1/images/edits` now have
  framework-independent JSON/multipart forwarding, official JSON/SSE usage
  validation, actual completed-image accounting, and no sticky-session cache.
- Subscription-account Images now has a framework-independent strict
  Images-to-Responses adapter with exact request-shape eligibility, real
  lifecycle conversion, exact tool-usage telemetry, and direct-account-only
  routing for unrepresentable public Images semantics.
- Native Frameless Live create and call-specific sideband forwarding now use
  exact-pool OAuth scheduling, the group Live switch, account-only long-lived
  concurrency leases, technical session ownership, real DeviceCheck
  attestation reuse, and raw text/binary WebSocket relay.
- The legacy gateway handlers, customer authentication implementation,
  customer caches/workers, and customer Ent schemas still require separation
  or deletion. OpenAI and Antigravity provider services still depend on Gin and
  block complete runtime assembly. Their presence is tracked as unfinished work,
  not as a runtime compatibility mechanism.

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
