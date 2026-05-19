#!/usr/bin/env bash
# conversation_describe_challenge.sh
#
# Round-271 paired-mutation deep-doc challenge for digital.vasic.conversation.
#
# Validates that:
#   1. The deep-doc ledger (docs/test-coverage.md) lists every exported
#      symbol from event_sourcing.go, infinite_context.go, and
#      context_compressor.go (Client + types surface).
#   2. The multi-locale fixture (tests/fixtures/conversation/payloads.json)
#      parses and contains at least 5 locales.
#   3. The multi-locale runner (challenges/runner/main.go) builds and
#      runs, byte-preserving non-ASCII payloads through ConversationEvent
#      ToJSON/FromJSON/Clone, CachedContext/ConversationSnapshot
#      JSON-roundtrip, ContextCompressor.Compress with a capturing
#      LLMClient, InfiniteContextEngine i18n-key surfacing, and
#      SetTranslator wiring + nil-reset semantics.
#   4. The README enumerates the round-271 anti-bluff guarantees.
#
# Paired-mutation invariant (CONST-035 + CONST-050(B)):
#   With --anti-bluff-mutate the script plants a deliberate symbol-rename
#   mutation in a tmp copy of the ledger (Compress ->
#   Compress_MUTATED), reruns validation, and asserts the gate
#   FAILS with exit 99. This proves the gate actually catches
#   ledger-vs-source drift instead of rubber-stamping it.
#
# Exit codes:
#   0  - gate PASS on clean tree
#   1  - gate FAIL on clean tree (real failure to fix)
#   99 - paired-mutation correctly detected (good - proves anti-bluff)
#   2  - usage / environment error

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

MUTATE=0
for arg in "$@"; do
    case "$arg" in
        --anti-bluff-mutate) MUTATE=1 ;;
        --help|-h)
            sed -n '1,32p' "$0"
            exit 0
            ;;
        *)
            echo "unknown argument: $arg" >&2
            exit 2
            ;;
    esac
done

PASS=0
FAIL=0
TOTAL=0

pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }

LEDGER="${MODULE_DIR}/docs/test-coverage.md"
FIXTURE="${MODULE_DIR}/tests/fixtures/conversation/payloads.json"
RUNNER="${MODULE_DIR}/challenges/runner/main.go"
README="${MODULE_DIR}/README.md"

LEDGER_WORK="${LEDGER}"
TMP_LEDGER=""
if [ "${MUTATE}" -eq 1 ]; then
    TMP_LEDGER="$(mktemp)"
    cp "${LEDGER}" "${TMP_LEDGER}"
    # Plant a rename so the symbol no longer matches what the source declares.
    sed -i 's/\bCompress\b/Compress_MUTATED/g' "${TMP_LEDGER}"
    LEDGER_WORK="${TMP_LEDGER}"
    echo "=== Conversation Describe Challenge (anti-bluff-mutate mode) ==="
else
    echo "=== Conversation Describe Challenge (clean mode) ==="
fi
echo ""

# Section 1: ledger presence and freshness
echo "Section 1: docs/test-coverage.md ledger"
if [ ! -f "${LEDGER_WORK}" ]; then
    fail "ledger missing at ${LEDGER_WORK}"
else
    pass "ledger present"
    if grep -q "round-271" "${LEDGER_WORK}"; then
        pass "ledger marked round-271"
    else
        fail "ledger missing round-271 marker"
    fi
    if grep -q "execution of tests and Challenges MUST guarantee" "${LEDGER_WORK}"; then
        pass "ledger carries Article XI §11.9 mandate"
    else
        fail "ledger missing Article XI §11.9 mandate"
    fi
fi

# Section 2: every exported package symbol appears in ledger.
echo ""
echo "Section 2: structural symbol cross-reference"

EXPECTED_SYMBOLS=(
    # event_sourcing.go
    "ConversationEvent" "ConversationEventType" "MessageData" "EntityData"
    "ContextData" "CompressionData" "DebateRoundData" "ConversationSnapshot"
    "EventStream" "NewConversationEvent" "ConversationEventFromJSON"
    "ConversationEventMessageAdded" "ConversationEventEntityExtracted"
    # infinite_context.go
    "InfiniteContextEngine" "ContextCache" "CachedContext"
    "NewInfiniteContextEngine" "ReplayConversation" "ReplayWithCompression"
    "GetConversationSnapshot" "SetTranslator"
    # context_compressor.go
    "ContextCompressor" "CompressionConfig" "CompressionStrategy"
    "CompressionStrategyHybrid" "CompressionStrategyWindowSummary"
    "CompressionStrategyEntityGraph" "CompressionStrategyFull"
    "DefaultCompressionConfig" "NewContextCompressor" "Compress"
    "LLMClient"
)

CHECKED=0
MISSING=0
for sym in "${EXPECTED_SYMBOLS[@]}"; do
    CHECKED=$((CHECKED + 1))
    if grep -qE "\\b${sym}\\b" "${LEDGER_WORK}"; then
        : # found
    else
        fail "ledger missing symbol ${sym}"
        MISSING=$((MISSING + 1))
    fi
done
if [ "${MISSING}" -eq 0 ]; then
    pass "all ${CHECKED} structural symbols cross-referenced in ledger"
fi

# Section 3: multi-locale fixture sanity
echo ""
echo "Section 3: multi-locale fixture"
if [ ! -f "${FIXTURE}" ]; then
    fail "fixture missing at ${FIXTURE}"
else
    pass "fixture present"
    LOCALE_COUNT=$(grep -oE '"locale":\s*"[^"]+"' "${FIXTURE}" | sort -u | wc -l)
    if [ "${LOCALE_COUNT}" -ge 5 ]; then
        pass "fixture covers ${LOCALE_COUNT} locales (>=5)"
    else
        fail "fixture covers only ${LOCALE_COUNT} locales (<5)"
    fi
fi

# Section 4: runner builds + runs against every section
echo ""
echo "Section 4: multi-locale runner build + run (real types + capturing LLMClient)"
if [ ! -f "${RUNNER}" ]; then
    fail "runner missing at ${RUNNER}"
else
    pass "runner source present"
    cd "${MODULE_DIR}"
    if go build -o /tmp/conv_round271_runner ./challenges/runner/ 2>/tmp/conv_build.log; then
        pass "runner builds"
        if /tmp/conv_round271_runner -fixtures "${FIXTURE}" > /tmp/conv_run.log 2>&1; then
            pass "runner exit 0 across every section + locale"
            # Per-locale + per-section PASS coverage
            if grep -q "PASS: \[Section1\]\[ToJSON/FromJSON/Clone\]\[sr\]" /tmp/conv_run.log; then
                pass "Section 1 Cyrillic (sr) ConversationEvent round-trip"
            else
                fail "Section 1 Cyrillic (sr) ConversationEvent missing"
            fi
            if grep -q "PASS: \[Section1\]\[ToJSON/FromJSON/Clone\]\[ja\]" /tmp/conv_run.log; then
                pass "Section 1 Japanese (ja) ConversationEvent round-trip"
            else
                fail "Section 1 Japanese (ja) ConversationEvent missing"
            fi
            if grep -q "PASS: \[Section1\]\[ToJSON/FromJSON/Clone\]\[ar\]" /tmp/conv_run.log; then
                pass "Section 1 Arabic (ar) ConversationEvent round-trip"
            else
                fail "Section 1 Arabic (ar) ConversationEvent missing"
            fi
            if grep -q "PASS: \[Section1\]\[ToJSON/FromJSON/Clone\]\[zh-CN\]" /tmp/conv_run.log; then
                pass "Section 1 Han (zh-CN) ConversationEvent round-trip"
            else
                fail "Section 1 Han (zh-CN) ConversationEvent missing"
            fi
            if grep -q "PASS: \[Section2\]\[CachedContext+Snapshot\]\[sr\]" /tmp/conv_run.log; then
                pass "Section 2 Cyrillic CachedContext+Snapshot JSON round-trip"
            else
                fail "Section 2 Cyrillic CachedContext missing"
            fi
            if grep -q "PASS: \[Section2\]\[CachedContext+Snapshot\]\[ar\]" /tmp/conv_run.log; then
                pass "Section 2 Arabic CachedContext+Snapshot JSON round-trip"
            else
                fail "Section 2 Arabic CachedContext missing"
            fi
            if grep -q "PASS: \[Section3\]\[Compress\]\[en\]" /tmp/conv_run.log; then
                pass "Section 3 Compressor English hybrid with capturing LLM"
            else
                fail "Section 3 Compress en missing"
            fi
            if grep -q "PASS: \[Section3\]\[Compress\]\[ja\]" /tmp/conv_run.log; then
                pass "Section 3 Compressor Japanese hybrid with capturing LLM"
            else
                fail "Section 3 Compress ja missing"
            fi
            if grep -q "PASS: \[Section3\]\[Compress\]\[zh-CN\]" /tmp/conv_run.log; then
                pass "Section 3 Compressor Han hybrid with capturing LLM"
            else
                fail "Section 3 Compress zh-CN missing"
            fi
            if grep -q "PASS: \[Section4\]\[GetConversationSnapshot\]\[en\]" /tmp/conv_run.log; then
                pass "Section 4 InfiniteContextEngine i18n-key surfacing"
            else
                fail "Section 4 GetConversationSnapshot missing"
            fi
            if grep -q "PASS: \[Section5\]\[SetTranslator\]\[en\]" /tmp/conv_run.log; then
                pass "Section 5 SetTranslator wiring captures conversation_* key (en)"
            else
                fail "Section 5 SetTranslator en missing"
            fi
            if grep -q "PASS: \[Section5\]\[SetTranslator\]\[sr\]" /tmp/conv_run.log; then
                pass "Section 5 SetTranslator wiring captures conversation_* key (sr)"
            else
                fail "Section 5 SetTranslator sr missing"
            fi
            if grep -q "PASS: \[Section5\]\[SetTranslator-nil\]\[en\]" /tmp/conv_run.log; then
                pass "Section 5 SetTranslator(nil) reset to NoopTranslator"
            else
                fail "Section 5 SetTranslator-nil en missing"
            fi
            if grep -q "PASS: \[Section5\]\[Compressor.SetTranslator\]" /tmp/conv_run.log; then
                pass "Section 5 Compressor.SetTranslator real + nil accepted"
            else
                fail "Section 5 Compressor.SetTranslator missing"
            fi
        else
            fail "runner exit non-zero - see /tmp/conv_run.log"
            sed -n '1,80p' /tmp/conv_run.log
        fi
    else
        fail "runner build failed - see /tmp/conv_build.log"
        sed -n '1,40p' /tmp/conv_build.log
    fi
    rm -f /tmp/conv_round271_runner
fi

# Section 5: README round-271 anti-bluff section
echo ""
echo "Section 5: README round-271 anti-bluff section"
if grep -q "Anti-bluff guarantees" "${README}"; then
    pass "README declares Anti-bluff guarantees"
else
    fail "README missing Anti-bluff guarantees section"
fi
if grep -q "round-271" "${README}"; then
    pass "README marked round-271"
else
    fail "README missing round-271 marker"
fi

# Cleanup mutated ledger if any
if [ -n "${TMP_LEDGER}" ]; then
    rm -f "${TMP_LEDGER}"
fi

echo ""
echo "=== Summary: ${PASS}/${TOTAL} PASS, ${FAIL} FAIL ==="

if [ "${MUTATE}" -eq 1 ]; then
    if [ "${FAIL}" -gt 0 ]; then
        echo "anti-bluff-mutate: gate correctly detected planted mutation (exit 99)"
        exit 99
    else
        echo "anti-bluff-mutate: gate FAILED to detect planted mutation - bluff!"
        exit 1
    fi
fi

if [ "${FAIL}" -gt 0 ]; then
    exit 1
fi
exit 0
