# CLAUDE.md - ConversationContext Module


## Definition of Done

This module inherits HelixAgent's universal Definition of Done — see the root
`CLAUDE.md` and `docs/development/definition-of-done.md`. In one line: **no
task is done without pasted output from a real run of the real system in the
same session as the change.** Coverage and green suites are not evidence.

### Acceptance demo for this module

```bash
# Kafka-sourced conversation replay with LLM-based compression
cd ConversationContext && GOMAXPROCS=2 nice -n 19 go test -count=1 -race -v \
  -run 'TestInfiniteContextEngine_ReplayWithCompression' ./...
```
Expect: PASS; events replay from the Kafka source (or in-memory test source), LLM summarization produces a compressed context window.


## Overview

`digital.vasic.conversation` is a generic, reusable Go module for conversation context management, compression, and infinite context via event sourcing. It provides tools for replaying conversations from Kafka event streams, caching conversation snapshots, and compressing conversations using LLM-based summarization.

**Module**: `digital.vasic.conversation` (Go 1.24+)

## Build & Test

```bash
go build ./...
go test ./... -count=1 -race
go test ./... -short              # Unit tests only
go test -bench=. ./...            # Benchmarks
```

## Code Style

- Standard Go conventions, `gofmt` formatting
- Imports grouped: stdlib, third-party, internal (blank line separated)
- Line length <= 100 chars
- Naming: `camelCase` private, `PascalCase` exported, acronyms all-caps
- Errors: always check, wrap with `fmt.Errorf("...: %w", err)`
- Tests: table-driven, `testify`, naming `Test<Struct>_<Method>_<Scenario>`

## Package Structure

| Package | Purpose |
|---------|---------|
| `conversation` (root) | Core types: InfiniteContextEngine, ContextCompressor, ContextCache, event sourcing, and conversation reconstruction |

## Key Types

- `InfiniteContextEngine`: Replays conversations from Kafka event streams with LRU caching
- `ContextCompressor`: Compresses conversations using LLM-based summarization with hybrid strategies (window summary, entity graph, fallback)
- `ContextCache`: LRU cache with TTL for conversation snapshots
- `ConversationEvent`: Event-sourced representation of conversation changes (message added, entity extracted, debate round, compression)
- `MessageData`, `EntityData`, `ContextData`: Core data structures

## Dependencies

- `digital.vasic.messaging`: Message broker abstraction for Kafka consumption
- `github.com/segmentio/kafka-go`: Direct Kafka access for event streaming
- `github.com/sirupsen/logrus`: Structured logging
- `github.com/stretchr/testify`: Testing framework

## Safety & Validation

- **Thread safety**: `InfiniteContextEngine` uses `sync.RWMutex` for concurrent access
- **Cache limits**: LRU eviction prevents unbounded memory growth
- **Context cancellation**: All long-running operations respect `context.Context`
- **Error handling**: Graceful degradation when LLM summarization fails

## Usage Example

```go
import (
    "context"
    "digital.vasic.conversation"
    "digital.vasic.messaging/pkg/broker"
)

// Create broker and compressor
kafkaConsumer := broker.NewKafkaBroker(config)
compressor := conversation.NewContextCompressor(llmClient, logger)

// Create infinite context engine
engine := conversation.NewInfiniteContextEngine(kafkaConsumer, compressor, logger)

// Replay conversation with compression
messages, compressionData, err := engine.ReplayWithCompression(
    ctx, "conv-123", 1000, // maxTokens
)
```

## Integration Seams

| Direction | Sibling modules |
|-----------|-----------------|
| Upstream (this module imports) | Messaging |
| Downstream (these import this module) | HelixLLM |

*Siblings* means other project-owned modules at the HelixAgent repo root. The root HelixAgent app and external systems are not listed here — the list above is intentionally scoped to module-to-module seams, because drift *between* sibling modules is where the "tests pass, product broken" class of bug most often lives. See root `CLAUDE.md` for the rules that keep these seams contract-tested.
