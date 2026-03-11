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

## License

MIT