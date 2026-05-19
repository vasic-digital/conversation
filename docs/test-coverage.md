# Test-Coverage Ledger — round-271

This ledger maps every exported symbol of `digital.vasic.conversation`
to the test or Challenge that exercises it with captured runtime
evidence. Per CONST-035, CONST-050(B), and the 2026-05-19 operator
mandate quoted below, no symbol may PASS without a corresponding
runtime-evidence exercise.

> Verbatim 2026-05-19 operator mandate: "all existing tests and
> Challenges do work in anti-bluff manner - they MUST confirm that
> all tested codebase really works as expected! We had been in
> position that all tests do execute with success and all
> Challenges as well, but in reality the most of the features does
> not work and can't be used! This MUST NOT be the case and
> execution of tests and Challenges MUST guarantee the quality, the
> completition and full usability by end users of the product!"

Operative rule (Article XI §11.9): **The bar for shipping is not
"tests pass" but "users can use the feature."** Every PASS in the
table below carries either a unit test, a paired-mutation gate, or
a challenge-runner section that produces positive runtime evidence —
no metadata-only / grep-only PASS counts.

## Module surface

`digital.vasic.conversation` ships two Go packages:

- **root `conversation`** — event-sourced conversation domain types
  + LRU cache + LLM-based compression + Kafka-replay engine:
  `ConversationEvent`, `ConversationEventType`, `MessageData`,
  `EntityData`, `ContextData`, `CompressionData`, `DebateRoundData`,
  `ConversationSnapshot`, `EventStream`, `NewConversationEvent`,
  `ConversationEventFromJSON`, the full set of
  `ConversationEvent<Type>` constants (`MessageAdded`,
  `MessageUpdated`, `MessageDeleted`, `Created`, `Completed`,
  `Archived`, `EntityExtracted`, `ContextUpdated`, `Compressed`,
  `DebateRound`, `DebateWinner`); `ContextCache` with `Get`/`Put`/
  `Clear`/`Size`; `CachedContext`; `InfiniteContextEngine` with
  `ReplayConversation`/`ReplayWithCompression`/
  `GetConversationSnapshot`/`SetTranslator`;
  `NewInfiniteContextEngine`; `ContextCompressor` with `Compress`/
  `SetTranslator`; `NewContextCompressor`; `CompressionConfig`;
  `CompressionStrategy` + `Hybrid`/`WindowSummary`/`EntityGraph`/
  `Full` constants; `DefaultCompressionConfig`; `LLMClient` interface.
- **`pkg/i18n`** — i18n contract: `Translator` interface (`T`,
  `TPlural`), `NoopTranslator` default that returns IDs verbatim.

## Symbol → exerciser map

### root `conversation` (`event_sourcing.go`)

| Symbol | Kind | Exercised by |
|--------|------|--------------|
| `ConversationEvent` | struct | runner Section 1 (5 locales — ToJSON+FromJSON+Clone byte-exact round-trip on Message+Entities+Context+Compression+DebateRound nested fields) + `event_sourcing_test.go` |
| `ConversationEventType` | type | runner Section 1 (event built with `ConversationEventMessageAdded`) + `event_sourcing_test.go` (TestConversationEventTypes) |
| `ConversationEventMessageAdded` | const | runner Section 1 (every locale builds an event with this type) |
| `ConversationEventEntityExtracted` | const | runner Section 1 (entities serialised + round-tripped) |
| `ConversationEventMessageUpdated` / `MessageDeleted` / `Created` / `Completed` / `Archived` / `ContextUpdated` / `Compressed` / `DebateRound` / `DebateWinner` | const | `event_sourcing_test.go` (TestConversationEventTypes — every constant referenced) |
| `MessageData` | struct | runner Section 1+2+3 (5 locales × user+assistant text byte-preserved through JSON + LLM Compress) + `event_sourcing_test.go` |
| `EntityData` | struct | runner Section 1 (Name byte-exact through JSON) + `infinite_context_test.go` |
| `ContextData` | struct | runner Section 1 (Clone deep-copy of KeyTopics/ActiveEntities slices) + Section 2 (ConversationSnapshot JSON round-trip) |
| `CompressionData` | struct | runner Section 1 (Clone deep-copy of PreservedEntities) + Section 3 (returned non-nil with OriginalMessages/CompressionRatio asserted per-locale) |
| `DebateRoundData` | struct | runner Section 1 (Clone preserves Response byte-exact across 5 locales) |
| `ConversationSnapshot` | struct | runner Section 2 (JSON marshal/unmarshal round-trip per-locale with Messages + Entities + Context preserved) + Section 4 (GetConversationSnapshot return type) |
| `EventStream` | struct | `event_sourcing_test.go` (field structure test) |
| `NewConversationEvent` | func | runner Section 1 (5 locale events constructed) + `event_sourcing_test.go` (TestNewConversationEvent) |
| `ConversationEventFromJSON` | func | runner Section 1 (5 locale round-trips) + `event_sourcing_test.go` (TestConversationEventFromJSON) |
| `ConversationEvent.ToJSON` | method | runner Section 1 (5 locales) + `event_sourcing_test.go` |
| `ConversationEvent.Clone` | method | runner Section 1 (deep-copy verified: mutate original → clone unchanged across all nested struct fields) |

### root `conversation` (`infinite_context.go`)

| Symbol | Kind | Exercised by |
|--------|------|--------------|
| `InfiniteContextEngine` | struct | runner Section 4+5 + `infinite_context_test.go` |
| `NewInfiniteContextEngine` | func | runner Section 4+5 (constructor returns non-nil even with nil broker/compressor/logger) + `infinite_context_test.go` (TestNewInfiniteContextEngine) |
| `InfiniteContextEngine.ReplayConversation` | method | `infinite_context_test.go` (replay paths covered) |
| `InfiniteContextEngine.ReplayWithCompression` | method | `infinite_context_test.go` (compression-required + compression-not-required + LLM-error paths) |
| `InfiniteContextEngine.GetConversationSnapshot` | method | runner Section 4 (no-Kafka path surfaces `conversation_replay_cache_miss_after_replay` i18n key) + `infinite_context_test.go` |
| `InfiniteContextEngine.SetTranslator` | method | runner Section 5 (capturing translator records keys; nil-reset to NoopTranslator both verified across 2 locales) |
| `ContextCache` | struct | `infinite_context_test.go` (TestContextCache — Get/Put/Size/Clear + LRU + TTL) |
| `ContextCache.Get` / `Put` / `Clear` / `Size` | methods | `infinite_context_test.go` (TestContextCache) — package-private constructor is the reason this isn't in the runner; consumer access is via the engine |
| `CachedContext` | struct | runner Section 2 (5 locales: Messages+Entities+Context byte-preservation through JSON round-trip) |

### root `conversation` (`context_compressor.go`)

| Symbol | Kind | Exercised by |
|--------|------|--------------|
| `ContextCompressor` | struct | runner Section 3+5 + `context_compressor_test.go` |
| `NewContextCompressor` | func | runner Section 3 (5 per-locale capturingLLMClient instances injected) + Section 5 (real+nil translator wiring) + `context_compressor_test.go` (TestNewContextCompressor) |
| `ContextCompressor.Compress` | method | runner Section 3 (5 locales × hybrid strategy, 60→13 messages, ratio<1, ≥1 LLM dispatch with locale bytes in prompt) + `context_compressor_test.go` (TestCompressionStrategies — all 4 strategies) |
| `ContextCompressor.SetTranslator` | method | runner Section 5 (real + nil reset accepted without panic) + `i18n_callsites_test.go` (TestContextCompressor — i18n callsite coverage) |
| `LLMClient` | interface | runner Section 3 (capturingLLMClient satisfies the interface and is injected via NewContextCompressor — the consumer's injection point, NOT a library-internal mock per CONST-050(A)) |
| `CompressionStrategy` | type | runner Section 3 (DefaultCompressionConfig().Strategy logged) |
| `CompressionStrategyHybrid` | const | runner Section 3 (default strategy = hybrid; per-locale Compress invocation) + `context_compressor_test.go` (TestCompressionStrategies) |
| `CompressionStrategyWindowSummary` | const | `context_compressor_test.go` (TestCompressionStrategies — strategy=window_summary invoked through Compress) |
| `CompressionStrategyEntityGraph` | const | `context_compressor_test.go` (TestCompressionStrategies — strategy=entity_graph invoked through Compress) |
| `CompressionStrategyFull` | const | `context_compressor_test.go` (TestCompressionStrategies — strategy=full invoked through Compress) |
| `CompressionConfig` | struct | runner Section 3 (DefaultCompressionConfig().Strategy/WindowSize/TargetRatio/LLMModel logged) |
| `DefaultCompressionConfig` | func | runner Section 3 (return value asserted non-degenerate) + `context_compressor_test.go` (TestDefaultCompressionConfig) |

### `pkg/i18n` (`translator.go`)

| Symbol | Kind | Exercised by |
|--------|------|--------------|
| `Translator` | interface | runner Section 5 (capturingTranslator satisfies the interface and is injected via engine.SetTranslator + compressor.SetTranslator) |
| `NoopTranslator` | struct | runner Section 5 (SetTranslator(nil) reset path; verbatim ID returned to error string asserted) + `translator_test.go` (TestNoopTranslator_T_ReturnsMsgIDVerbatim + plural + nil-args variants) |
| `NoopTranslator.T` | method | runner Section 4+5 (engine's i18n key surfaces in cache-miss error string verbatim) + `translator_test.go` |
| `NoopTranslator.TPlural` | method | `translator_test.go` (TestNoopTranslator_TPlural_ReturnsMsgIDVerbatim) |

## Test runs (round-271 evidence captured)

### `GOMAXPROCS=2 go test -count=1 -race -short ./...`

```
ok  	digital.vasic.conversation	(race ~1s)
ok  	digital.vasic.conversation/pkg/i18n	(race ~1s)
```

Both packages pass with `-race` enabled — no data-race detected at
the cache mutex, the engine RWMutex, or the compressor's translator
field.

### `challenges/runner/main.go -fixtures tests/fixtures/conversation/payloads.json`

```
=== Round-271 Conversation Challenge Runner ===
... 22 PASS lines across 5 sections, 5 locales ...
=== Summary: 22 PASS, 0 FAIL ===
```

Per-locale runtime evidence captured:

- **Section 1** — 5 PASS: ConversationEvent ToJSON + FromJSON +
  Clone byte-exact round-trip per locale on Message+Entities+
  Context+Compression+DebateRound nested fields. Deep-copy
  semantics verified by mutating the original and asserting the
  clone is untouched.
- **Section 2** — 5 PASS: CachedContext + ConversationSnapshot
  JSON marshal/unmarshal byte-exact per locale with Messages +
  Entities + Context + KeyTopics + ActiveEntities all preserved.
  Rune-floor sanity assertion guards against accidentally-empty
  fixtures.
- **Section 3** — 6 PASS (1 config + 5 per-locale): hybrid Compress
  shrinks 60 messages → 13 per locale, ratio in (0,1], capturing
  LLMClient was invoked >=1 time AND received the locale's non-ASCII
  bytes verbatim in its prompt.
- **Section 4** — 1 PASS: InfiniteContextEngine constructor returns
  non-nil with nil broker/compressor/logger; GetConversationSnapshot
  on a no-Kafka path surfaces the `conversation_replay_cache_miss_after_replay`
  i18n key verbatim (NoopTranslator default). Locale-iteration is
  skipped here because the engine behaviour is locale-independent and
  each call costs ~10s of Kafka-timeout; full locale-multiplicity is
  proven in Sections 1-3.
- **Section 5** — 5 PASS: capturingTranslator records >=1
  `conversation_*` key per engine GetConversationSnapshot call
  across en + sr locales; SetTranslator(nil) resets to NoopTranslator
  without panic and the verbatim ID still flows to the error string;
  Compressor.SetTranslator accepts both a real translator AND nil
  without panic.

### `bash challenges/scripts/conversation_describe_challenge.sh`

Clean mode exit 0; `--anti-bluff-mutate` exit 99 (paired mutation
correctly detected — ledger-vs-source drift caught when the gate
plants a `Compress -> Compress_MUTATED` rename in a tmp copy of
this ledger and the structural cross-reference check trips).

## Anti-bluff invariants

This round addresses every taxonomy entry in CLAUDE.md §"Bluff
taxonomy":

- **Wrapper bluff** — the describe-challenge wrapper uses PASS/FAIL
  counters with a separate `set -uo pipefail` guard, never inline
  arithmetic on a command that prints + exits non-zero.
- **Contract bluff** — every public method on `InfiniteContextEngine`,
  `ContextCompressor`, every public function on `ConversationEvent`
  (constructor + ToJSON + FromJSON + Clone), and every exported type
  listed above is exercised by a runtime test or challenge section.
  The ledger surface is closed and audited symbol-by-symbol.
- **Structural bluff** — no `check_file_exists` PASS without a
  paired functional assertion. Every PASS carries either a rune
  count, a message count, a compression ratio, a JSON byte-equality,
  an LLM dispatch count, an i18n-key match, or a non-nil sentinel.
- **Comment bluff** — the README's `## Anti-bluff guarantees`
  section is enforced by `conversation_describe_challenge.sh`
  Section 5.
- **Skip bluff** — no `t.Skip()` in the unit tests; the runner has
  no `if false { … }` dead branches; the locale-skipping in Section 4
  is documented in-source as a runtime-cost trade-off, NOT a bluff
  (Section 4's assertion is locale-independent by package design).

## Cross-reference to constitutional anchors

| Anchor | Layer | How honoured |
|--------|-------|--------------|
| CONST-035 / Article XI §11.9 | end-user-usability | every PASS line carries runtime evidence (locale, rune count, ratio, dispatch count, i18n key) |
| CONST-046 | no-hardcoded-content | runner asserts i18n keys (`conversation_*`) surface verbatim through NoopTranslator — proves the i18n contract is wired through the engine + compressor public surfaces, not bypassed |
| CONST-050(A) | no-fakes-beyond-unit-tests | runner uses only the public conversation API; the capturingLLMClient + capturingTranslator are the consumer's injected dependencies, NOT library-internal mocks |
| CONST-050(B) | 100%-test-type coverage | unit tests + challenge runner + paired-mutation gate together cover unit + integration-style + meta-test layers |
| CONST-051(B) | submodule decoupling | runner imports only `digital.vasic.conversation` + `digital.vasic.conversation/pkg/i18n` — no consumer-project reach-in |
| CONST-053 | .gitignore | `.gitignore` covers `/bin/`, `*.test`, `coverage.out`, `*.log`, `.env*`, secrets, IDE state, tmp + OS-state files (round-271 enrichment) |

The 2026-05-19 operator mandate is preserved verbatim above and in
the runner's package doc comment.
