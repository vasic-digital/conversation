# AGENTS.md - ConversationContext Module Multi-Agent Coordination Guide

## Overview

This document provides guidance for AI agents (Claude Code, Copilot, Cursor, etc.) working on the `digital.vasic.conversation` module. It defines responsibilities, boundaries, and coordination protocols to prevent conflicts when multiple agents operate concurrently.

## Module Identity

- **Module path**: `digital.vasic.conversation`
- **Language**: Go 1.24+
- **Dependencies**: `digital.vasic.messaging`, `github.com/segmentio/kafka-go`, `github.com/sirupsen/logrus`, `github.com/stretchr/testify`
- **Packages**: Single package `conversation` (root)

## Package Ownership Boundaries

### `conversation` -- Core Conversation Context Management

- **Scope**: `InfiniteContextEngine`, `ContextCompressor`, `ContextCache`, `ConversationEvent`, `MessageData`, `EntityData`, `ContextData`, event reconstruction, compression strategies.
- **Owner concern**: Any agent modifying the core data structures (`MessageData`, `EntityData`) must update all dependent methods (reconstruction, compression, caching).
- **Thread safety**: `InfiniteContextEngine` uses `sync.RWMutex`. All new methods MUST acquire appropriate locks. `ContextCache` uses its own mutex.
- **Dependencies**: Depends on `digital.vasic.messaging` for broker abstraction and `kafka-go` for direct Kafka access.

## Dependency Graph

```
conversation -> digital.vasic.messaging
conversation -> github.com/segmentio/kafka-go
```

The module has no internal package dependencies (single package). External dependencies are stable interfaces.

## Agent Coordination Rules

### 1. Interface Changes

If you modify `InfiniteContextEngine` public API:
- Update all callers in tests (`infinite_context_test.go`)
- Consider backward compatibility - existing callers in HelixAgent may need updates
- Update `ContextCompressor` if compression logic changes

If you modify `ContextCompressor` public API:
- Update `InfiniteContextEngine` usage
- Update tests

### 2. Struct Field Changes

Adding fields to `MessageData`, `EntityData`, `ContextData`:
- Check JSON tags follow existing `snake_case` convention with `omitempty` where appropriate
- Update `reconstructFromEvents` to handle new fields
- Update `calculateContext` if fields affect context calculation
- Update compression strategies if fields affect summarization

Adding fields to `ConversationEvent`:
- Update all event type switch statements (`reconstructFromEvents`)
- Update event serialization/deserialization if needed

### 3. Concurrency Safety

- `InfiniteContextEngine`: Uses `sync.RWMutex` (`mu` field). Read operations use `RLock`/`RUnlock`. Write operations use `Lock`/`Unlock`.
- `ContextCache`: Has its own `sync.RWMutex`. Follow same locking discipline.
- `ContextCompressor`: Stateless, no locking required.

Rules:
- Never hold a lock while calling external functions (e.g., LLM client, Kafka reads)
- Always return copies of internal data, never pointers to stored objects
- Use `defer mu.Unlock()` pattern

### 4. Testing Standards

- **Framework**: `github.com/stretchr/testify` (assert + require)
- **Naming**: `Test<Struct>_<Method>_<Scenario>` (e.g., `TestContextCache_PutAndGet`)
- **Style**: Table-driven tests with `tests` slice and `t.Run` subtests
- **Mocking**: Use `mockBroker` for messaging abstraction, `mockLLMClient` for LLM calls
- **Run all tests**: `go test ./... -count=1 -race`

### 5. Adding New Features

To add new compression strategies:
1. Add new strategy constant to `CompressionStrategy` enum
2. Implement strategy logic in `ContextCompressor.compressWithStrategy`
3. Add tests for new strategy
4. Update `DefaultCompressionConfig` if needed

To add new event types:
1. Add new `ConversationEventType` constant
2. Update `reconstructFromEvents` to handle new event type
3. Add corresponding field to `ConversationEvent` struct
4. Add tests for event reconstruction

### 6. File Ownership

| File | Primary Concern | Cross-Package Impact |
|------|----------------|---------------------|
| `infinite_context.go` | InfiniteContextEngine, event replay, caching | HIGH -- main entry point |
| `context_compressor.go` | Compression strategies, LLM summarization | MEDIUM -- used by engine |
| `event_sourcing.go` | ConversationEvent types, reconstruction | MEDIUM -- used by engine |
| `infinite_context_test.go` | Test suite for InfiniteContextEngine | LOW |
| `context_compressor_test.go` | Test suite for ContextCompressor | LOW |

## Build and Validation Commands

```bash
# Full validation
go build ./...
go test ./... -count=1 -race
go vet ./...
gofmt -l .

# Single test suite
go test -v ./...

# Benchmarks (if any)
go test -bench=. ./...
```

## Commit Conventions

- Use Conventional Commits: `feat(conversation): add new compression strategy`
- Scope: `conversation` (single package)
- Use `docs` scope for documentation-only changes
- Run `gofmt` and `go vet` before every commit

## Integration with HelixAgent

The module is integrated into HelixAgent via `internal/conversation` adapter. Changes to public API may require updates to:
- `internal/conversation/` adapter (if exists)
- `internal/services/context_manager.go`
- Any direct callers in HelixAgent

Always test integration by building HelixAgent after making changes.