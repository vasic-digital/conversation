# digital.vasic.conversation

A generic, reusable Go module for conversation context management, compression, and infinite context via event sourcing. Provides tools for replaying conversations from Kafka event streams, caching conversation snapshots, and compressing conversations using LLM-based summarization.

## Features

- **Infinite Context via Event Sourcing**: Replay full conversation history from Kafka event streams
- **LLM-Based Compression**: Hybrid compression strategies (window summary, entity graph, fallback) to fit conversations within token limits
- **LRU Caching with TTL**: Efficient caching of conversation snapshots with automatic eviction
- **Thread-Safe Operations**: `sync.RWMutex` protection for concurrent access
- **Graceful Degradation**: Fallback strategies when LLM summarization fails
- **Comprehensive Event Types**: Support for message added, entity extracted, debate round, compression events

## Installation

```bash
go get digital.vasic.conversation
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "digital.vasic.conversation"
    "digital.vasic.messaging/pkg/broker"
)

func main() {
    // Create a message broker (e.g., Kafka consumer)
    kafkaConfig := &broker.Config{
        Brokers:  []string{"localhost:9092"},
        ClientID: "conversation-replay",
    }
    kafkaConsumer := broker.NewKafkaBroker(kafkaConfig)
    
    // Create context compressor with LLM client (optional)
    compressor := conversation.NewContextCompressor(nil, nil) // No LLM client for basic usage
    
    // Create infinite context engine
    engine := conversation.NewInfiniteContextEngine(kafkaConsumer, compressor, nil)
    
    // Replay a conversation from Kafka event stream
    ctx := context.Background()
    messages, err := engine.ReplayConversation(ctx, "conv-123")
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Replayed %d messages\n", len(messages))
    for _, msg := range messages {
        fmt.Printf("- %s: %s\n", msg.Role, msg.Content)
    }
    
    // Replay with compression (for token-limited contexts)
    compressedMessages, compressionData, err := engine.ReplayWithCompression(
        ctx, "conv-123", 1000, // maxTokens = 1000
    )
    if err != nil {
        log.Fatal(err)
    }
    
    if compressionData != nil {
        fmt.Printf("Compressed from %d to %d messages (ratio: %.2f)\n",
            compressionData.OriginalMessages,
            len(compressedMessages),
            compressionData.CompressionRatio)
    }
}
```

## Core Components

### InfiniteContextEngine
Replays conversations from Kafka event streams with LRU caching. Key methods:
- `ReplayConversation(ctx, conversationID)`: Full replay from event stream
- `ReplayWithCompression(ctx, conversationID, maxTokens)`: Replay with LLM compression
- `GetConversationSnapshot(ctx, conversationID)`: Get cached snapshot

### ContextCompressor
Compresses conversations using hybrid strategies:
- **Window Summary**: Summarizes message windows with LLM
- **Entity Graph**: Preserves entity relationships
- **Fallback**: Preserves most recent messages when LLM fails

### ContextCache
LRU cache with TTL for conversation snapshots. Automatic eviction when cache exceeds max size or entries expire.

### ConversationEvent Types
- `ConversationEventMessageAdded`: New message added to conversation
- `ConversationEventEntityExtracted`: Entity extracted from messages
- `ConversationEventDebateRound`: Debate round in multi-LLM debate
- `ConversationEventCompressed`: Compression applied to conversation

## Configuration

Default compression configuration can be customized:

```go
config := conversation.DefaultCompressionConfig()
config.MaxTokens = 2000
config.Strategy = conversation.CompressionStrategyEntityGraph
config.WindowSize = 10
compressor := conversation.NewContextCompressor(llmClient, logger, config)
```

## Thread Safety

- `InfiniteContextEngine` uses `sync.RWMutex` for concurrent access
- `ContextCache` has its own mutex protection
- `ContextCompressor` is stateless and safe for concurrent use

## Testing

```bash
# Run all tests with race detection
go test ./... -count=1 -race

# Run unit tests only
go test ./... -short

# Run specific test suites
go test -v -run TestContextCache ./...
go test -v -run TestInfiniteContextEngine ./...
go test -v -run TestContextCompressor ./...

# Benchmarks (if any)
go test -bench=. ./...
```

## Integration with HelixAgent

This module is extracted from HelixAgent's `internal/conversation` package. In HelixAgent, it's used for:

- **Infinite context window**: Replaying conversation history for long-running discussions
- **Debate context preservation**: Storing debate rounds as conversation events
- **Token limit compliance**: Compressing conversations to fit within model token limits

## Anti-bluff guarantees (round-271)

Per Article XI §11.9 and CONST-035 / CONST-050(B), this module ships
a deep-doc + multi-locale runner + paired-mutation Challenge that
prove every advertised capability actually works for the end user —
not just that the test suite reports green.

**What the round-271 evidence stack guarantees:**

1. **Symbol-by-symbol audit.** `docs/test-coverage.md` enumerates
   every exported symbol from `event_sourcing.go`,
   `infinite_context.go`, `context_compressor.go`, and `pkg/i18n` —
   each row paired with either a unit test, a Challenge runner
   section, or both. The describe-challenge script
   (`challenges/scripts/conversation_describe_challenge.sh`)
   cross-checks that every expected symbol literally appears in
   the ledger; ledger-vs-source drift trips the gate.
2. **5-locale bilingual fixture.** `tests/fixtures/conversation/payloads.json`
   ships `en`, `sr` (Cyrillic), `ja` (Japanese), `ar` (Arabic, RTL),
   `zh-CN` (Han) entries with non-ASCII user messages, assistant
   messages, entity names, and summary markers. The runner asserts
   byte-exact survival of every locale's payload through
   ConversationEvent ToJSON/FromJSON/Clone, CachedContext +
   ConversationSnapshot JSON marshal/unmarshal, and the
   ContextCompressor's LLM dispatch path.
3. **Real LLMClient injection (no library mock).** The runner's
   `capturingLLMClient` is the *consumer's* injection — exactly
   what a production consumer would supply to `NewContextCompressor`.
   It is NOT a library-internal fake; CONST-050(A) is honoured. The
   runner asserts the locale's prompt bytes reach the LLM client
   verbatim, not via a logger or a string-format intermediary.
4. **Real Translator injection (CONST-046).** The runner's
   `capturingTranslator` is the *consumer's* injection through the
   engine's `SetTranslator` + the compressor's `SetTranslator`. The
   runner asserts the engine actually requests `conversation_*`
   i18n keys at runtime (rather than emitting hardcoded English
   strings) — proves the round-128 CONST-046 migration is intact.
5. **Paired-mutation gate.** Running the describe-challenge with
   `--anti-bluff-mutate` plants a deliberate
   `Compress → Compress_MUTATED` rename in a tmp copy of the
   ledger and re-runs the symbol cross-check. The gate exits 99
   (correctly detected planted mutation). A gate that fails to trip
   on the planted mutation would itself be a bluff; the meta-test
   guards the gate.
6. **No skip bluffs.** The unit suite has no bare `t.Skip()`. The
   runner's only locale-skip (Section 4 + 5 sub-iterations) is
   documented in-source as a runtime-cost trade-off backed by the
   package design — the engine's i18n surface is locale-independent.

**To re-verify everything:**

```bash
# 1. Unit tests pass with -race.
GOMAXPROCS=2 go test -count=1 -race -short ./...

# 2. Runner exits 0 with full PASS coverage across 5 locales.
go run ./challenges/runner -fixtures tests/fixtures/conversation/payloads.json

# 3. Challenge gate clean.
bash challenges/scripts/conversation_describe_challenge.sh

# 4. Paired-mutation gate trips.
bash challenges/scripts/conversation_describe_challenge.sh --anti-bluff-mutate ; \
  test $? -eq 99 && echo "anti-bluff gate intact" || echo "GATE BLUFF"
```

> Verbatim 2026-05-19 operator mandate: "all existing tests and
> Challenges do work in anti-bluff manner - they MUST confirm that
> all tested codebase really works as expected! We had been in
> position that all tests do execute with success and all
> Challenges as well, but in reality the most of the features does
> not work and can't be used! This MUST NOT be the case and
> execution of tests and Challenges MUST guarantee the quality, the
> completition and full usability by end users of the product!"

## License

MIT