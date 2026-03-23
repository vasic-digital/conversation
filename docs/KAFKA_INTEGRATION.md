# ConversationContext Module - Kafka Integration

**Module:** `digital.vasic.conversation`
**Last Updated:** March 2026

---

## Overview

The ConversationContext module uses Apache Kafka as the persistent event store for conversation history. This document describes the Kafka integration architecture, topic design, message format, and replay mechanics.

---

## Kafka Topic Design

### Primary Topic

| Topic | Purpose |
|-------|---------|
| `conversation-events` | All conversation lifecycle events |

All conversation events for all conversations are published to this single topic. Events are filtered by `ConversationID` during replay. In a production deployment with high throughput, this can be partitioned by conversation ID hash for parallelism.

### Event Routing

Events are published as JSON-serialized `ConversationEvent` structs. The message key is the `ConversationID`, ensuring all events for a conversation land in the same partition (when using partitioned topics).

```
Key:   conversation-id
Value: JSON(ConversationEvent)
```

---

## Publishing Events

Events are published to Kafka through the `digital.vasic.messaging` broker abstraction. In HelixAgent, the conversation context manager publishes events whenever the conversation state changes.

### Example: Publishing a Message Event

```go
import (
    "digital.vasic.conversation"
    "digital.vasic.messaging/pkg/broker"
)

func publishMessageAdded(
    kafkaProducer broker.MessageBroker,
    conversationID, userID, nodeID string,
    message conversation.MessageData,
) error {
    event := conversation.NewConversationEvent(
        conversation.ConversationEventMessageAdded,
        nodeID,
        conversationID,
        userID,
    )
    event.Message = &message

    data, err := event.ToJSON()
    if err != nil {
        return fmt.Errorf("failed to serialize event: %w", err)
    }

    return kafkaProducer.Publish(ctx, "conversation-events", conversationID, data)
}
```

### Example: Publishing an Entity Extraction Event

```go
func publishEntitiesExtracted(
    kafkaProducer broker.MessageBroker,
    conversationID, userID, nodeID string,
    entities []conversation.EntityData,
) error {
    event := conversation.NewConversationEvent(
        conversation.ConversationEventEntityExtracted,
        nodeID,
        conversationID,
        userID,
    )
    event.Entities = entities

    data, err := event.ToJSON()
    if err != nil {
        return fmt.Errorf("failed to serialize event: %w", err)
    }

    return kafkaProducer.Publish(ctx, "conversation-events", conversationID, data)
}
```

### Example: Publishing a Debate Round Event

```go
func publishDebateRound(
    kafkaProducer broker.MessageBroker,
    conversationID, userID, nodeID string,
    round conversation.DebateRoundData,
) error {
    event := conversation.NewConversationEvent(
        conversation.ConversationEventDebateRound,
        nodeID,
        conversationID,
        userID,
    )
    event.DebateRound = &round

    data, err := event.ToJSON()
    if err != nil {
        return fmt.Errorf("failed to serialize event: %w", err)
    }

    return kafkaProducer.Publish(ctx, "conversation-events", conversationID, data)
}
```

---

## Replaying Events

The `InfiniteContextEngine` replays events from Kafka to reconstruct conversation state.

### Replay Flow

```
1. Check ContextCache for cached conversation
   |
   +-- Cache HIT: Return cached messages immediately
   |
   +-- Cache MISS:
       |
       2. Create a Kafka reader for "conversation-events" topic
       3. Seek to FirstOffset (beginning of topic)
       4. Read messages with 10-second timeout
       5. Filter events by ConversationID
       6. Sort events by SequenceNumber
       7. Reconstruct conversation from events
       8. Calculate context data (token counts, entity counts)
       9. Store in ContextCache
       10. Return reconstructed messages
```

### Kafka Reader Configuration

```go
reader := kafka.NewReader(kafka.ReaderConfig{
    Brokers:   []string{"localhost:9092"},
    Topic:     "conversation-events",
    GroupID:   "infinite-context-<conversationID>",
    Partition: 0,
    MinBytes:  1,
    MaxBytes:  10e6, // 10MB max fetch
})
```

**Key Configuration Notes:**
- `GroupID` is unique per conversation to allow independent offset tracking.
- The reader seeks to `FirstOffset` to read all events from the beginning.
- A 10-second read timeout prevents indefinite blocking.
- A safety limit of 10,000 events prevents memory exhaustion for very long conversations.

### Replay with Compression

When a conversation exceeds the model's token limit, `ReplayWithCompression` adds a compression step:

```
1. ReplayConversation() -- full history
2. Count tokens
3. If tokens <= maxTokens: return as-is
4. Retrieve cached entities
5. ContextCompressor.Compress() -- apply compression strategy
6. Return compressed messages + CompressionData
```

---

## Kafka Configuration

### Broker Configuration

| Parameter | Default | Recommended Production |
|-----------|---------|----------------------|
| Brokers | `localhost:9092` | Cluster of 3+ brokers |
| Topic partitions | 1 | Number of concurrent conversations / 100 |
| Replication factor | 1 | 3 (for durability) |
| Retention | 7 days | Based on conversation lifetime |
| Max message size | 1MB | 10MB (for large conversations) |

### Consumer Configuration

| Parameter | Value | Notes |
|-----------|-------|-------|
| `MinBytes` | 1 | Respond as soon as data is available |
| `MaxBytes` | 10MB | Accommodate large batches |
| Read timeout | 10s | Prevents indefinite blocking |
| Safety limit | 10,000 events | Prevents memory exhaustion |

---

## Event Stream Management

### EventStream Type

The module provides an `EventStream` struct for managing conversation event streams:

```go
type EventStream struct {
    StreamID       string
    ConversationID string
    UserID         string
    SessionID      string
    StartTime      time.Time
    EndTime        time.Time
    Events         []*ConversationEvent
    EventCount     int
    Checkpoints    []time.Time
}
```

### ConversationSnapshot

A point-in-time snapshot of a conversation's full state:

```go
type ConversationSnapshot struct {
    SnapshotID     string
    ConversationID string
    UserID         string
    SessionID      string
    Timestamp      time.Time
    NodeID         string
    SequenceNumber int64
    Messages       []MessageData
    Entities       []EntityData
    Context        *ContextData
    Metadata       map[string]interface{}
}
```

Snapshots are created via `InfiniteContextEngine.GetConversationSnapshot()` and can be used for debugging, auditing, or transferring conversation state between nodes.

---

## Production Considerations

### Topic Partitioning

For high-throughput deployments, partition the `conversation-events` topic by conversation ID hash. This ensures:
- All events for a conversation are in the same partition (ordering guarantee).
- Multiple conversations can be replayed in parallel across partitions.

### Compaction

Consider enabling log compaction on the conversation events topic to retain only the latest snapshot for completed conversations, reducing storage costs while maintaining the ability to replay active conversations.

### Monitoring

Monitor the following Kafka consumer metrics:
- **Consumer lag** -- How far behind the consumer is from the latest event.
- **Replay latency** -- Time to replay a full conversation.
- **Cache hit rate** -- Percentage of replays served from cache.

### Error Handling

- **Kafka unavailable:** The `fetchConversationEvents` method logs warnings and returns an empty event list. The calling code handles the empty result gracefully.
- **Malformed events:** JSON parse failures are logged and skipped. The conversation is reconstructed from valid events only.
- **Offset seek failure:** Logged as a warning; the reader continues from the current offset rather than failing completely.
