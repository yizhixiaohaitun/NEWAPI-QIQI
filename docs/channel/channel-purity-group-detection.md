# Channel purity group detection

## Goal

Channel purity detection is performed **inside a configured channel group**. A group has one trusted baseline channel and one or more target channels. Every target is assessed independently against the baseline; the product does not collapse targets into a group-wide purity score.

The existing quick probe remains a manual connectivity diagnostic. It is not the primary purity score and cannot replace paired production-like samples.

## Detection cycle

The scheduler runs every 5–10 minutes (configurable within that range):

1. Read enabled groups and validate that each group has exactly one enabled baseline.
2. Select privacy-safe request candidates from actual traffic buckets. A bucket is defined by group, requested model, mapped upstream model family, protocol, stream mode, tool/structured-output flags, and a coarse prompt-token range.
3. Replay the same normalized candidate to the baseline and one target using internal detector credentials and a recursion guard. Requests containing files, secrets, personal data, arbitrary tool side effects, or unsupported bodies are never replayed.
4. Extract response features in memory before protocol normalization. Persist only allow-listed anonymous features and token counts/ranges; do not persist request/response text, reasoning, tool arguments, credentials, account identifiers, or arbitrary headers.
5. Compare each target pair with the baseline and write an independent assessment containing sample count, similarity, anomaly evidence, confidence, and status.
6. Alert only after debouncing across consecutive windows. Recover only after consecutive healthy windows.

A baseline failure must produce `BASELINE_UNAVAILABLE`, not a false alert on every target. Detector failures are reported as `DETECTOR_ERROR` and are not attributed to a channel.

## Two complementary detectors

### 1. Protocol and response fingerprint

Compare stable response characteristics, including:

- HTTP status class and media type class;
- top-level JSON paths and value types;
- response/object/id prefix class;
- model family and finish status;
- usage field presence and arithmetic validity;
- streaming event taxonomy, order, terminal event, and usage placement;
- allow-listed response-header presence bits;
- model-specific evidence profiles, such as Codex Responses event graphs, reasoning/item shape, turn-state/header presence, and versioned preset signature identifiers.

Content-style or hidden-preset signatures are supporting evidence, not a single-sample proof. A Codex preset match is stored as a versioned signature ID and boolean result, never as matched source text.

### 2. Paired output-token distribution

The same safe request is sent to baseline and target. Output token counts are **not expected to be identical**. Comparison is performed within matching request buckets using robust statistics:

- paired token ratio and log-ratio;
- median and median absolute deviation (MAD);
- quantile interval overlap (for example P10–P90);
- outlier rate relative to the baseline distribution;
- confidence derived from effective paired sample count and dispersion.

Token length is weak evidence on its own. It can raise suspicion when a persistent distribution shift agrees with protocol, mapping, or model-specific evidence, but a single token mismatch cannot trigger an alert.

There is no universal international standard that defines an exact output-token interval for a model. The implementation instead follows international observability conventions for token metrics and standard reference-versus-current drift methods, calibrated per group/model/request bucket.

## Assessment contract

Each non-baseline channel receives its own record:

- `sample_count` and `baseline_sample_count`;
- `similarity` (0–100);
- `confidence` (0–100);
- protocol/schema similarity;
- output-token distribution similarity;
- machine-readable anomaly reason codes and evidence levels;
- window start/end and detector/rule version;
- status.

Statuses:

- `BASELINE_UNAVAILABLE`: baseline cannot provide a valid paired reference;
- `NO_TRAFFIC`: no eligible real request candidate in the window;
- `LOW_SAMPLE`: samples exist but cannot support a stable conclusion;
- `WARMING_UP`: enough data is being accumulated across initial windows;
- `HEALTHY`: evidence is sufficiently consistent with the baseline;
- `SUSPECT`: meaningful drift exists but alert debounce/confidence is not met;
- `ALERT`: high-confidence drift persisted for the configured consecutive windows;
- `DETECTOR_ERROR`: capture, replay, or comparison failed internally.

`UNKNOWN`-class states must never be counted as healthy.

## Alert policy

Recommended defaults:

- evaluate every 5 minutes;
- require at least 20 valid paired samples for high-confidence statistical conclusions;
- allow a lower threshold for deterministic protocol violations;
- enter `ALERT` after 3 consecutive suspicious windows, unless a high-grade deterministic violation permits immediate alerting;
- recover after 2 consecutive healthy windows;
- keep transport failure, rate limiting, and upstream outage evidence separate from channel-purity evidence;
- never disable a channel automatically in the first implementation.

## Privacy and performance

The capture path is fail-open for user traffic:

- response observation uses a bounded streaming observer rather than `io.ReadAll`;
- capture errors and queue saturation never block or alter the user response;
- raw body fragments are discarded immediately after allow-listed feature extraction;
- samples are asynchronously batch-written with a short TTL;
- replay candidates are held only for a short TTL and must pass a strict safety filter;
- internal detector calls carry a recursion guard and never become new candidates;
- sampling and per-channel/model rate limits cap CPU, memory, database, and upstream cost.

## Reference conventions

The design borrows conventions rather than copying another project's product model:

- OpenTelemetry GenAI semantic conventions for request/response model and input/output token metric naming: <https://opentelemetry.io/docs/specs/semconv/gen-ai/> and <https://opentelemetry.io/docs/specs/semconv/registry/attributes/gen-ai/>.
- Arize Phoenix/OpenInference token accounting conventions and the need to avoid double-counting parent/child spans: <https://arize.com/docs/phoenix/tracing/how-to-tracing/cost-tracking>.
- LiteLLM callback and hook patterns as a reference for sidecar observation, while keeping capture at the shared upstream HTTP boundary so endpoint-specific adapters cannot bypass it: <https://docs.litellm.ai/docs/observability/custom_callback> and <https://docs.litellm.ai/docs/proxy/call_hooks>.
- Evidently's reference-versus-current drift approach, configurable thresholds, and distribution tests: <https://docs.evidentlyai.com/metrics/explainer_drift>.

These references do not define an official model-authenticity certificate. The product conclusion is deliberately phrased as “consistent with the configured trusted baseline” or “drift detected from the configured trusted baseline.”
