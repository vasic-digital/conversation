# ConversationContext Module - Architecture

**Module:** `digital.vasic.conversation`
**Version:** 1.0.0
**Last Updated:** March 2026

---

## Design Philosophy

The ConversationContext module provides **infinite conversation context** through event sourcing and intelligent compression. It is designed to:

1. **Overcome token limits** -- Replay full conversation history from Kafka event streams.
2. **Compress intelligently** -- LLM-based summarization preserves key information.
3. **Cache efficiently** -- LRU cache with TTL avoids redundant Kafka reads.
4. **Degrade gracefully** -- Fallback strategies when LLM summarization fails.
5. **Be thread-safe** -- All components use proper synchronization for concurrent access.

---

## High-Level Architecture

```
+-------------------------------------------------------------------+
|                        Application Layer                           |
|                                                                    |
|  Debate Service / Chat Handler / Context Manager                   |
+-------------------------------------------------------------------+
         |
         v
+-------------------------------------------------------------------+
|                    InfiniteContextEngine                            |
|                                                                    |
|  ReplayConversation()     -- Full history from Kafka               |
|  ReplayWithCompression()  -- Compressed to fit token limits        |
|  GetConversationSnapshot() -- Point-in-time snapshot               |
+-------------------------------------------------------------------+
         |                    |                    |
         v                    v                    v
+------------------+  +------------------+  +------------------+
|  Kafka Consumer  |  | ContextCompressor|  |  ContextCache    |
|  (event replay)  |  | (LLM-based)      |  |  (LRU + TTL)    |
+------------------+  +------------------+  +------------------+
         |                    |
         v                    v
+------------------+  +------------------+
| ConversationEvent|  |   LLMClient      |
| (event sourcing) |  | (summarization)  |
+------------------+  +------------------+
```

---

## Event Sourcing Model

The module uses event sourcing as its core data model. Instead of storing the current state of a conversation, it stores a sequence of immutable events in Kafka. The current state is reconstructed by replaying these events in order.

### Event Types

| Event Type | Description | Data Carried |
|------------|-------------|--------------|
| `conversation.message.added` | New message added | `MessageData` (role, content, tokens) |
| `conversation.message.updated` | Message edited | `MessageData` |
| `conversation.message.deleted` | Message removed | Message ID |
| `conversation.created` | Conversation started | Conversation metadata |
| `conversation.completed` | Conversation ended | Final state |
| `conversation.archived` | Conversation archived | Archive metadata |
| `conversation.entity.extracted` | Entities found | `[]EntityData` |
| `conversation.context.updated` | Context recalculated | `ContextData` |
| `conversation.compressed` | Compression applied | `CompressionData` |
| `conversation.debate.round` | Debate round completed | `DebateRoundData` |
| `conversation.debate.winner` | Debate winner selected | Winner metadata |

### Event Structure

Every event carries:

```go
type ConversationEvent struct {
    EventID        string                // Unique event identifier
    EventType      ConversationEventType // Type discriminator
    Timestamp      time.Time             // When the event occurred
    NodeID         string                // Source node (for distributed systems)
    SequenceNumber int64                 // Ordering guarantee
    ConversationID string                // Which conversation
    SessionID      string                // Session context
    UserID         string                // Who triggered it
    Message        *MessageData          // Message payload (if applicable)
    Entities       []EntityData          // Extracted entities (if applicable)
    Context        *ContextData          // Context snapshot (if applicable)
    Compression    *CompressionData      // Compression info (if applicable)
    DebateRound    *DebateRoundData      // Debate data (if applicable)
    Metadata       map[string]interface{} // Extensible metadata
}
```

### Reconstruction Algorithm

Events are replayed in `SequenceNumber` order. The reconstruction logic processes each event type:

1. **MessageAdded** -- Appends the message to the conversation.
2. **EntityExtracted** -- Merges entities into an entity map (deduped by EntityID).
3. **DebateRound** -- Converts debate round data into an assistant message with model and token metadata.
4. **Compressed** -- Skipped during reconstruction (compression is applied separately).

---

## Compression Strategies

When a conversation exceeds the model's token limit, the `ContextCompressor` applies intelligent compression using LLM-based summarization.

### Strategy: Window Summary

Divides the conversation into windows of configurable size (default: 10 messages). Each window is summarized into a single system message using the LLM. Recent messages (last 25% of conversation, minimum 50) are preserved verbatim.

```
[Messages 1-10]  -> [Summary 1]
[Messages 11-20] -> [Summary 2]
[Messages 21-30] -> [Summary 3]
[Messages 31-40] -- preserved verbatim --
```

### Strategy: Entity Graph

Preserves messages that mention known entities while summarizing entity-free messages. This maintains critical context around key entities (people, systems, decisions) while compressing filler.

```
[Message: "The API uses REST"]     -> Summarized (no entities)
[Message: "Claude handles auth"]   -> Preserved (entity: Claude)
[Message: "Yes, that's correct"]   -> Summarized (no entities)
[Message: "Redis caches tokens"]   -> Preserved (entity: Redis)
```

### Strategy: Full

Creates a single comprehensive summary of the entire conversation (5-10 sentences) plus the 20% most recent messages. This is the most aggressive compression, used as a last resort.

### Strategy: Hybrid (Default)

A multi-pass approach that applies strategies in order of increasing aggressiveness until the conversation fits within the token limit:

```
Pass 1: Entity Graph compression
  -> If still over limit:
Pass 2: Window Summary on already-compressed output
  -> If still over limit:
Pass 3: Full summary of original conversation
```

### Fallback Behavior

When the LLM client is unavailable (`nil`), the compressor falls back to:
- Window summary: `"[Summary of N messages]"` placeholder
- Full summary: `"[Summary of N messages with M entities]"` placeholder

This ensures the system continues to function (with reduced quality) when LLM summarization is unavailable.

---

## Caching Layer

The `ContextCache` provides an LRU cache with TTL to avoid redundant Kafka replays.

### Cache Behavior

| Parameter | Default | Description |
|-----------|---------|-------------|
| `maxSize` | 100 | Maximum number of cached conversations |
| `ttl` | 30 minutes | Time-to-live for cache entries |

**Cache Hit:** Returns the cached `CachedContext` containing messages, entities, and computed context data. Increments the access counter.

**Cache Miss:** Triggers a full Kafka replay, then stores the result.

**Eviction:** When the cache is full, the oldest entry (by `CachedAt` timestamp) is evicted. Expired entries (older than TTL) are removed on access.

### CachedContext Structure

```go
type CachedContext struct {
    ConversationID string
    Messages       []MessageData
    Entities       []EntityData
    Context        *ContextData
    CachedAt       time.Time
    AccessCount    int
}
```

---

## Data Types

### MessageData

```go
type MessageData struct {
    MessageID string    // Unique message identifier
    Role      string    // "user", "assistant", "system"
    Content   string    // Message text content
    Model     string    // LLM model used (for assistant messages)
    Tokens    int       // Token count (0 = estimate from content length)
    CreatedAt time.Time // When the message was created
}
```

### EntityData

```go
type EntityData struct {
    EntityID   string                 // Unique entity identifier
    Type       string                 // Entity type (person, system, concept, etc.)
    Name       string                 // Entity name
    Value      string                 // Entity value (optional)
    Confidence float64                // Extraction confidence (0-1)
    Context    string                 // Context in which entity was found
    Properties map[string]interface{} // Additional properties
}
```

### ContextData

```go
type ContextData struct {
    MessageCount      int      // Total messages in conversation
    TotalTokens       int64    // Estimated total tokens
    EntityCount       int      // Number of unique entities
    CompressedCount   int      // Number of compressed messages
    CompressionRatio  float64  // Compression ratio (if compressed)
    KeyTopics         []string // Key topics discussed
    ActiveEntities    []string // Currently relevant entities
    ContextWindow     int      // Model context window size
    ContextUsageRatio float64  // Percentage of context window used
}
```

### CompressionData

```go
type CompressionData struct {
    CompressionID       string    // Unique compression identifier
    CompressionType     string    // Strategy used
    OriginalMessages    int       // Messages before compression
    CompressedMessages  int       // Messages after compression
    OriginalTokens      int64     // Tokens before compression
    CompressedTokens    int64     // Tokens after compression
    CompressionRatio    float64   // Compressed/Original ratio
    PreservedEntities   []string  // Entities preserved through compression
    LLMModel            string    // Model used for summarization
    CompressionDuration int64     // Duration in milliseconds
    CompressedAt        time.Time // When compression occurred
}
```

---

## Token Estimation

When `MessageData.Tokens` is 0 (not provided by the LLM), the module estimates token count using:

```
estimated_tokens = len(content) / 4
```

This approximation (4 characters per token) is consistent with typical English text tokenization and provides a conservative estimate suitable for context window management.

---

## Thread Safety

| Component | Synchronization | Notes |
|-----------|----------------|-------|
| `InfiniteContextEngine` | `sync.RWMutex` | Protects Kafka replay and cache access |
| `ContextCache` | `sync.RWMutex` | Protects the cache map; eviction runs under write lock |
| `ContextCompressor` | Stateless | No synchronization needed; safe for concurrent use |

---

## File Structure

```
ConversationContext/
  event_sourcing.go          -- Event types, ConversationEvent, data structures
  infinite_context.go        -- InfiniteContextEngine, ContextCache
  context_compressor.go      -- ContextCompressor, compression strategies
  *_test.go                  -- Test files
```
