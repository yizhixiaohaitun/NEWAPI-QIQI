# Upstream Scan 2026-07-14

## Scope

- Local branch: `main` / `qiqi/main`
- Official remote: `origin` (`https://github.com/QuantumNous/new-api.git`)
- Anchor commit: `722d0366b727b82fced878af902e48363626b2fb`
- Anchor subject: `fix(classic): fix classic web build failures`
- Anchor time: `2026-07-04 08:47:32 +0800`
- Scanned official head: `7c28993f6` (`origin/main`)
- Scanned official version: `v1.0.0-rc.21-2-g7c28993f6`
- Latest tag in range: `v1.0.0-rc.21`
- Commit range: `722d0366..origin/main`

## Scan Summary

- Official commits scanned: 85
- Changed files: 387
- Insertions: 31946
- Deletions: 8659
- Backend/relay/service scope: 176 files, 16572 insertions, 5074 deletions
- Frontend scope: 185 files, 15194 insertions, 3420 deletions
- Workflow/env/dependency scope: 10 files, 60 insertions, 41 deletions

## New Official Tags

| Tag | Commit | Date | Notes |
| --- | --- | --- | --- |
| `v1.0.0-rc.17` | `45f0484dc` | 2026-07-06 | Build date DNS fix |
| `v1.0.0-rc.18` | `c9943d37a` | 2026-07-07 | Billing quantity validation and saturation fixes |
| `v1.0.0-rc.19` | `5cbb7b0be` | 2026-07-07 | Architecture docs update |
| `v1.0.0-rc.19-i18nfix.1` | `8f31b3059` | 2026-07-07 | Locale formatting standardization |
| `v1.0.0-rc.19-i18nfix.2` | `becc18e30` | 2026-07-07 | Chinese locale detection mapping |
| `v1.0.0-rc.20` | `6ce7305cd` | 2026-07-07 | GPT-5.6 token ratios |
| `v1.0.0-rc.21` | `bde9b2f44` | 2026-07-11 | Unset price models tab hardening |

## Main Change Areas

### 1. Security, Auth, and Account Hardening

Recommended priority: P0.

- `df087b022` adds SSRF protection across HTTP clients and validation paths. It touches `common/ssrf_protection.go`, `service/http_client.go`, `service/protected_fetch_client.go`, download, webhook, user notification, video proxy, and MJ proxy paths.
- `56dbaab1d` adds opt-in Secure session cookie support through env/config and middleware helpers.
- `5fc35e28a` hardens account email and password update behavior, adds explicit user errors, and adds regression tests.
- `bed4a3f91` trims username whitespace and validates username input.
- `0d5995eb6` allows read-only access for non-disabled tokens.
- `00f1cbb6d` bumps `golang.org/x/crypto` from `0.51.0` to `0.52.0`.

Merge value: high. These are security and account correctness fixes.

Merge risk: medium. SSRF touches shared HTTP client behavior and must be checked against existing provider callbacks, file/image fetch flows, webhooks, and any QIQI-specific proxy logic.

### 2. Billing, Quota, Pricing, and Audit Safety

Recommended priority: P0/P1.

- `d0bd8aac7`, `c9943d37a`, `bae799ccb`, `48b7f4918`, `621927f71`, and `d9595831b` harden quota math, quantity validation, saturating conversions, quota saturation logging, and pre-consume failure behavior.
- `48068ce92` bills OpenAI `cache_write_tokens` at cache-creation price and clamps negative values to zero.
- `92d3c9d18` bounds uncached prompt remainder and forwards compact `prompt_cache_key`.
- `043720f9b` fixes task differential settlement quota handling and Alibaba video duration behavior.
- `3fbad6a72` adds default token estimate for tiered expression pre-consume.
- `90fa6fe6b` makes wallet reward transfers honor configured quota units.
- `fc1259f58` improves handling of other ratios in `PriceData`.
- `6ce7305cd` adds token ratios for GPT-5.6 models.
- `97bbb7c8c` improves dynamic pricing calculations with group selection support.
- `394b023db` keeps group ratio input as string draft so decimal typing is not broken.
- `81808d241` and `f2c7cd33c` remove sample special usable groups leaking into pricing pages.
- `8283df169`, `bde9b2f44`, `93e936f70`, and `7c28993f6` add and harden the unset-price-models tab and ensure it lists only channel models.

Merge value: high. This directly affects charging correctness, auditability, and admin pricing operations.

Merge risk: high. QIQI has local changes in usage logs and response accounting paths. The upstream billing changes should be merged with focused tests around text quota, image billing, task billing, tiered settlement, and usage-log rendering.

### 3. Relay, Protocol Conversion, and Provider Compatibility

Recommended priority: P1, but split into its own merge round.

- `c36418c86` is the largest change. It introduces a new `service/relayconvert` registry and internal conversion modules for OpenAI Chat, OpenAI Responses, Claude Messages, and Gemini Chat conversions. It also changes advanced custom routing and usage conversion.
- `153d7f01a` avoids stale stream writes after client disconnect.
- `269e4ff39` improves image stream handling with client disconnect logic and billing adjustments.
- `dad57a6bb` syncs Codex channel fields.
- `4ae341756` exposes field passthrough controls for Codex in channel settings.
- `57865fc1f` restores default channel connection paste.
- `c36418c86`, `48068ce92`, and `92d3c9d18` also touch OpenAI Responses conversion and compact response handling.

Merge value: high. This is where official has most of the protocol and provider behavior improvements.

Merge risk: very high. This overlaps with QIQI's local OpenAI/Responses compatibility work. The dry-run merge reports a real conflict in `relay/channel/api_request.go`; other related files auto-merge but still need semantic review:

- `controller/relay.go`
- `dto/openai_response.go`
- `relay/channel/openai/adaptor.go`
- `relay/channel/openai/relay_responses.go`
- `relay/channel/openai/relay_responses_compact.go`
- `relay/common/relay_info.go`
- `relay/helper/common.go`
- `relay/responses_handler.go`
- `service/log_info_generate.go`

### 4. Admin Operations: Subscription and System Instance Management

Recommended priority: P1.

- `9b93d61b7` adds admin quota reset actions for subscriptions with API, model, router, UI, and i18n coverage.
- `4e570389d` switches subscription reset locking to GORM v2 row locking.
- `a72e5082e` adds stale system instance cleanup actions.
- `e40061965` enhances stale instance handling and adjusts theme colors.

Merge value: medium-high. Useful admin operations and safer transactional behavior.

Merge risk: medium. Backend changes are relatively isolated, but UI and i18n changes overlap with local frontend changes.

### 5. Default Frontend UX and Admin Productivity

Recommended priority: P1/P2.

- `8739c05c0` adds manual column resizing to the channels table.
- `928b47507` and `4823417cf` add a Playground chat parameter panel.
- `4645ad9df` keeps Playground model selector lists in sync.
- `7a2b9d86e` enhances model search with status and sync filters.
- `489c04584`, `43783286e`, and `0cb741d8d` optimize upstream price sync tables and polish sync dialog layout.
- `6bbddb104` adds timing metrics display for stream logs and localization.
- `308e3e347` adds task log details and polishes themed data views, but much of the broad UI refactor was later reverted by `337169e0a` and `1b1b23d1d`.
- `28e0115a0` prevents browser translation from mutating React roots.
- `3a876d6f3` redirects authenticated users away from sign-up page.
- `2f91d8ccb` syncs home iframe theme and language.
- `17465b855` fixes dark/light switching under Shadow DOM isolated rendering.
- `2281c9e3d` refines mobile user cards.
- `12603a776` adds redemption status filtering and cleanup actions, plus mobile redemption list updates.
- `1ae757475` and `f52b52b16` align dynamic pricing style with log details dialog sections.
- `fc26b88fd` improves group ratio editor visibility rules and JSON parsing.

Merge value: medium-high. Several changes improve daily admin work.

Merge risk: medium-high. Actual conflicts exist in usage-log columns/types and i18n JSON files. UI changes should be split by feature area instead of one large merge.

### 6. i18n and Locale Handling

Recommended priority: P2.

- `d1abf78ec` adds zh-TW localization for the new UI and creates `web/default/src/i18n/locales/zh-TW.json`.
- `8f31b3059` standardizes locale formatting for Intl APIs.
- `becc18e30` maps Chinese language detection more robustly.
- Many feature commits add new strings across `en`, `zh`, `fr`, `ja`, `ru`, and `vi`.

Merge value: medium. Important for complete UI behavior, especially if frontend features are merged.

Merge risk: high but mechanical. Dry-run merge reports content conflicts in all existing default frontend locale files: `en`, `fr`, `ja`, `ru`, `vi`, and `zh`.

### 7. Classic Frontend, Build, Release, and Miscellaneous

Recommended priority: P2/P3.

- `45f0484dc` fixes build date DNS behavior.
- `8bc4bf1d6` adds cosign for signing manifests and updates workflow permissions.
- `c36418c86` adjusts several release/build workflows.
- `246d62aa5` removes dead files resurrected by the v1.0 launch commit.
- Classic frontend gets deprecation banner, theme helper, i18n updates, and build config updates.
- `b2a890e75` fixes Fontsource asset resolution across workspace layouts.
- README files get minor star count updates.

Merge value: medium for build/security supply-chain changes, lower for docs/assets.

Merge risk: medium. There is a real conflict in `web/classic/rsbuild.config.ts`, likely because QIQI has local classic build changes.

## Dry-Run Merge Results

Command used:

```bash
git merge-tree --write-tree --name-only main origin/main
```

Result: merge simulation exits with conflicts, but worktree remains clean.

Actual content conflict files:

```text
relay/channel/api_request.go
web/classic/rsbuild.config.ts
web/classic/src/constants/channel-affinity-template.constants.js
web/default/src/features/system-settings/general/channel-affinity/constants.ts
web/default/src/features/usage-logs/components/columns/common-logs-columns.tsx
web/default/src/features/usage-logs/types.ts
web/default/src/i18n/locales/en.json
web/default/src/i18n/locales/fr.json
web/default/src/i18n/locales/ja.json
web/default/src/i18n/locales/ru.json
web/default/src/i18n/locales/vi.json
web/default/src/i18n/locales/zh.json
```

Files changed by both upstream and QIQI since the anchor:

```text
controller/relay.go
dto/openai_response.go
relay/channel/api_request.go
relay/channel/openai/adaptor.go
relay/channel/openai/relay_responses.go
relay/channel/openai/relay_responses_compact.go
relay/common/relay_info.go
relay/helper/common.go
relay/responses_handler.go
service/log_info_generate.go
setting/operation_setting/channel_affinity_setting.go
web/classic/rsbuild.config.ts
web/classic/src/constants/channel-affinity-template.constants.js
web/default/src/features/system-settings/general/channel-affinity/constants.ts
web/default/src/features/usage-logs/components/columns/common-logs-columns.tsx
web/default/src/features/usage-logs/components/usage-logs-mobile-card.tsx
web/default/src/features/usage-logs/types.ts
web/default/src/i18n/locales/en.json
web/default/src/i18n/locales/fr.json
web/default/src/i18n/locales/ja.json
web/default/src/i18n/locales/ru.json
web/default/src/i18n/locales/vi.json
web/default/src/i18n/locales/zh.json
```

## Recommended Merge Queue

### Batch A: Security and account correctness

Recommended action: merge first.

Candidate commits:

- `df087b022` SSRF protection
- `56dbaab1d` Secure session cookies
- `5fc35e28a` account email/password hardening
- `bed4a3f91` username trim and validation
- `0d5995eb6` non-disabled token read-only access
- `00f1cbb6d` x/crypto dependency bump

Expected conflicts: low to medium. SSRF requires behavior verification.

### Batch B: Quota, billing, and pricing safety

Recommended action: merge after Batch A.

Candidate commits:

- `d0bd8aac7`
- `c9943d37a`
- `bae799ccb`
- `48b7f4918`
- `621927f71`
- `d9595831b`
- `48068ce92`
- `92d3c9d18`
- `043720f9b`
- `3fbad6a72`
- `90fa6fe6b`

Expected conflicts: medium. Watch usage-log model/types and QIQI accounting compatibility.

### Batch C: Relay conversion and advanced custom routing

Recommended action: merge as a dedicated branch, not mixed with UI or i18n.

Candidate commits:

- `c36418c86`
- `153d7f01a`
- `269e4ff39`
- `dad57a6bb`
- `4ae341756`
- `57865fc1f`

Expected conflicts: high. `relay/channel/api_request.go` has a real conflict, and several OpenAI Responses files require semantic review even when Git auto-merges them.

### Batch D: Admin operations

Recommended action: merge if admin subscription/system-instance workflows are wanted.

Candidate commits:

- `9b93d61b7`
- `4e570389d`
- `a72e5082e`
- `e40061965`

Expected conflicts: low to medium for backend, medium for frontend/i18n.

### Batch E: Frontend productivity features

Recommended action: evaluate feature-by-feature.

Candidate commits:

- `8739c05c0` channel table column resizing
- `928b47507` / `4823417cf` Playground parameter panel
- `4645ad9df` Playground model selector sync
- `7a2b9d86e` model search filters
- `489c04584` / `43783286e` / `0cb741d8d` upstream price sync table optimization
- `8283df169` / `bde9b2f44` / `93e936f70` / `7c28993f6` unset price models tab
- `6bbddb104` stream timing metrics
- `12603a776` redemption status filtering and cleanup

Expected conflicts: medium-high, mostly logs, i18n, and system settings constants.

### Batch F: i18n, classic, build, and polish

Recommended action: merge after functional features are decided.

Candidate commits:

- `d1abf78ec`
- `8f31b3059`
- `becc18e30`
- `45f0484dc`
- `8bc4bf1d6`
- `b2a890e75`
- `246d62aa5`

Expected conflicts: high for locale JSON, medium for classic build config.

## Full Commit List

| Commit | Date | Author | Subject |
| --- | --- | --- | --- |
| `86021d8ed` | 2026-07-04 | CaIon | Refine default web UI and backend sync handling |
| `12603a776` | 2026-07-04 | CaIon | fix(redemption): add status filtering and cleanup action |
| `bed4a3f91` | 2026-07-04 | CaIon | fix(user): trim whitespace from username and validate input |
| `4ae341756` | 2026-07-04 | Seefs | fix(channels): show field passthrough controls for Codex (#5902) |
| `f52b52b16` | 2026-07-04 | feitianbubu | fix: align dynamic pricing style with log details dialog sections |
| `1ae757475` | 2026-07-04 | 同語 | fix: align dynamic pricing style with log details dialog sections |
| `81808d241` | 2026-07-04 | feitianbubu | fix: remove sample special usable groups leaking into pricing page |
| `5fc35e28a` | 2026-07-05 | CaIon | fix(user): harden account email and password handling |
| `0d5995eb6` | 2026-07-05 | CaIon | fix(auth): allow read-only access for non-disabled tokens |
| `56dbaab1d` | 2026-07-05 | CaIon | feat(session): support opt-in Secure session cookies |
| `4a64b8707` | 2026-07-05 | CaIon | test(user): cover self-service password update guard |
| `2281c9e3d` | 2026-07-05 | CaIon | fix(web): refine mobile user cards |
| `043720f9b` | 2026-07-06 | feitianbubu | fix: 任务差额结算后 quota 和阿里视频时长优化 (#5923) |
| `2f91d8ccb` | 2026-07-06 | G.RQ | fix(web): sync home iframe theme and language (#5917) |
| `17465b855` | 2026-07-06 | olwater | fix(html): 修复 Shadow DOM 隔离渲染下深浅色模式无法自动切换的问题 (#5890) |
| `1e11dfcfb` | 2026-07-06 | CaIon | feat(user): better messages for redeem failures |
| `df087b022` | 2026-07-06 | CaIon | feat(ssrf): implement SSRF protection in HTTP clients and validation functions |
| `3a876d6f3` | 2026-07-06 | 乾L | fix(web): redirect authenticated users away from sign-up page (#5910) |
| `1e80ce03e` | 2026-07-06 | Gravirei | feat: optimize legacy top-up warning banner copy (#5851) (#5855) |
| `fc26b88fd` | 2026-07-06 | CaIon | feat(group): enhance group ratio editor with improved visibility rules and JSON parsing |
| `153d7f01a` | 2026-07-06 | Seefs | fix: avoid stale stream writes after client disconnect (#5710) |
| `45f0484dc` | 2026-07-06 | Seefs | Fix/build date dns error (#5945) |
| `d0bd8aac7` | 2026-07-07 | CaIon | fix(billing): validate quantity parameters and harden quota calculations |
| `c9943d37a` | 2026-07-07 | CaIon | fix(billing): extend quantity validation and saturating conversions to remaining paths |
| `bae799ccb` | 2026-07-07 | CaIon | fix(billing): surface quota saturation events for admin auditing |
| `70ea899e3` | 2026-07-07 | CaIon | fix(model): centralize row locking in transactional flows |
| `d1abf78ec` | 2026-07-07 | Meow Tech Open Source by NovaMeow | Localized new ui to zh-TW (#5942) |
| `9b93d61b7` | 2026-07-07 | Seefs | feat(subscription): add admin quota reset actions (#5952) |
| `48b7f4918` | 2026-07-07 | CaIon | fix(billing): adjust quota calculation to prevent exceeding int32 limits |
| `5cbb7b0be` | 2026-07-07 | CaIon | docs: update system architecture requirements |
| `8f31b3059` | 2026-07-07 | CaIon | fix(i18n): standardize locale formatting for Intl APIs |
| `becc18e30` | 2026-07-07 | CaIon | fix(i18n): add language detection mapping for Chinese locales |
| `3fbad6a72` | 2026-07-07 | CaIon | fix(price): add default token estimate for tiered expression pre-consume |
| `8bc4bf1d6` | 2026-07-07 | CaIon | feat(docker): add cosign for signing manifests and update permissions |
| `2f5f6ba84` | 2026-07-07 | CaIon | feat: prepare for 5.6 |
| `394b023db` | 2026-07-07 | feitianbubu | fix: keep group ratio input as string draft to allow decimal typing (#5995) |
| `fc1259f58` | 2026-07-07 | CaIon | refactor(price): improve handling of other ratios in PriceData |
| `a72e5082e` | 2026-07-07 | Seefs | feat(system-info): add stale instance cleanup actions (#5953) |
| `90fa6fe6b` | 2026-07-07 | A_Words | fix(wallet): honor configured quota units for reward transfers (#5808) |
| `6ce7305cd` | 2026-07-07 | CaIon | feat(price): add token ratios for GPT-5.6 models |
| `57865fc1f` | 2026-07-08 | CaIon | fix: restore default channel connection paste |
| `6a437a337` | 2026-07-08 | CaIon | feat(oauth): add OAuth callback URL display and copy functionality |
| `28e0115a0` | 2026-07-08 | Rāna(Bass Ver.) | fix(web): prevent browser translation from mutating React roots (#5963) |
| `97bbb7c8c` | 2026-07-08 | CaIon | feat(pricing): enhance dynamic pricing calculations with group selection support |
| `8739c05c0` | 2026-07-08 | zuiho | feat(web): 支持渠道列表手动调整列宽 (#5948) |
| `df01273b9` | 2026-07-09 | zuiho | fix(web): let resized tables fill available width (#6031) |
| `a79f96919` | 2026-07-09 | CaIon | fix(affiliate): update referral message |
| `4645ad9df` | 2026-07-09 | QuentinHsu | fix(playground): keep model selector lists in sync |
| `246d62aa5` | 2026-07-09 | feitianbubu | chore: remove dead files resurrected by v1.0 launch commit (#6041) |
| `928b47507` | 2026-07-09 | QuentinHsu | feat(playground): add chat parameter settings panel |
| `4e570389d` | 2026-07-10 | Seefs | fix: use GORM v2 row locking for subscription resets (#6057) |
| `e8596cab7` | 2026-07-10 | feitianbubu | fix: allow adding custom model names that differ only by case |
| `4823417cf` | 2026-07-10 | 同語 | feat(playground): add playground parameter settings panel (#6044) |
| `d3b01b483` | 2026-07-10 | 同語 | fix: allow adding model names that differ only by case in multi-select (#6061) |
| `f2c7cd33c` | 2026-07-10 | 同語 | fix: remove sample special usable groups leaking into pricing page (#5906) |
| `489c04584` | 2026-07-10 | QuentinHsu | perf(model-pricing): optimize upstream price sync table |
| `43783286e` | 2026-07-10 | QuentinHsu | fix(model-pricing): polish sync channel dialog layout |
| `6869cd94b` | 2026-07-11 | QuentinHsu | perf(web): align table badge spacing |
| `0cb741d8d` | 2026-07-11 | 同語 | perf(model-pricing): optimize upstream price sync tables (#6092) |
| `262ab9312` | 2026-07-11 | t0ng7u | style(web): unify design system across default frontend |
| `0918bdb49` | 2026-07-11 | t0ng7u | refactor(web): consolidate design-system primitives and responsive data views |
| `9d1ca545e` | 2026-07-11 | t0ng7u | refactor(web): refine data-table cards and pricing page layout |
| `ca971413e` | 2026-07-11 | 乾L | fix(web): allow user-activated top navigation for custom home iframe (#5955) |
| `00f1cbb6d` | 2026-07-11 | dependabot[bot] | chore(deps): bump golang.org/x/crypto from 0.51.0 to 0.52.0 (#6096) |
| `dad57a6bb` | 2026-07-11 | Seefs | fix: sync codex field (#6018) |
| `b2a890e75` | 2026-07-11 | t0ng7u | fix: Fontsource asset resolution across workspace layouts |
| `621927f71` | 2026-07-10 | CaIon | fix(billing): reject saturated pre-consume quota |
| `d9595831b` | 2026-07-10 | CaIon | fix(billing): improve quota handling and error reporting for pre-consume operations |
| `269e4ff39` | 2026-07-11 | CaIon | feat(image): enhance image stream handling with client disconnect logic and billing adjustments |
| `308e3e347` | 2026-07-11 | t0ng7u | feat(web): polish themed data views and add task log details |
| `ad900bbba` | 2026-07-11 | t0ng7u | Merge remote-tracking branch 'origin/main' |
| `337169e0a` | 2026-07-11 | CaIon | revert: undo t0ng7u UI design-system refactor |
| `1b1b23d1d` | 2026-07-11 | CaIon | revert: restore StatusBadge horizontal padding |
| `6bbddb104` | 2026-07-11 | CaIon | feat(timing): add timing metrics display for stream logs and enhance localization |
| `162f87925` | 2026-07-11 | CaIon | feat: update theme colors |
| `e40061965` | 2026-07-11 | CaIon | feat: enhance stale instance handling and update theme colors |
| `1250fb2eb` | 2026-07-11 | CaIon | fix: adjust margin for StatusBadge component in logs columns |
| `c36418c86` | 2026-07-11 | Calcium-Ion | feat: enhance text protocol conversion and advanced custom routing (#5825) |
| `48068ce92` | 2026-07-11 | CaIon | feat: bill OpenAI cache_write_tokens at cache-creation price with zero clamp |
| `92d3c9d18` | 2026-07-11 | CaIon | fix: bound uncached remainder by prompt-max(cached,write) and forward compact prompt_cache_key |
| `7a2b9d86e` | 2026-07-11 | CaIon | feat: enhance model search functionality with status and sync filters |
| `8283df169` | 2026-07-11 | feitianbubu | feat: add unset price models tab to model pricing settings (#6124) |
| `bde9b2f44` | 2026-07-11 | CaIon | fix: harden unset price models tab batch copy, feedback, and memo equality |
| `93e936f70` | 2026-07-11 | feitianbubu | fix: list only channel models in unset price models tab |
| `7c28993f6` | 2026-07-12 | 同語 | fix: list only channel models in unset price models tab (#6126) |

## Commands Used

```bash
git fetch origin --tags
git describe --tags --long --always origin/main
git rev-list --count 722d0366b727b82fced878af902e48363626b2fb..origin/main
git log --reverse --date=short --format='%h%x09%ad%x09%an%x09%s' 722d0366b727b82fced878af902e48363626b2fb..origin/main
git diff --shortstat 722d0366b727b82fced878af902e48363626b2fb..origin/main
git diff --stat 722d0366b727b82fced878af902e48363626b2fb..origin/main
git merge-tree --write-tree --name-only main origin/main
```
