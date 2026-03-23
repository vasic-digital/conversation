# ConversationContext Module - Examples

**Module:** `digital.vasic.conversation`
**Last Updated:** March 2026

---

## Creating the Infinite Context Engine

### Basic Setup

```go
import (
    "context"
    "fmt"

    "digital.vasic.conversation"
    "digital.vasic.messaging/pkg/broker"
    "github.com/sirupsen/logrus"
)

func setupContextEngine() *conversation.InfiniteContextEngine {
    // Create a Kafka consumer via the messaging module
    kafkaConfig := &broker.Config{
        Brokers:  []string{"localhost:9092"},
        ClientID: "conversation-replay",
    }
    kafkaConsumer := broker.NewKafkaBroker(kafkaConfig)

    // Create a context compressor (with or without LLM client)
    compressor := conversation.NewContextCompressor(nil, nil) // nil = fallback mode

    // Create the engine
    logger := logrus.New()
    engine := conversation.NewInfiniteContextEngine(kafkaConsumer, compressor, logger)

    return engine
}
```

### With LLM-Based Compression

```go
// Implement the LLMClient interface
type MyLLMClient struct {
    provider llmprovider.LLMProvider
}

func (c *MyLLMClient) Complete(ctx context.Context, prompt string, maxTokens int) (string, int, error) {
    req := &models.LLMRequest{
        Prompt: prompt,
        ModelParams: models.ModelParameters{
            MaxTokens:   maxTokens,
            Temperature: 0.3, // Low temperature for summarization
        },
    }

    resp, err := c.provider.Complete(ctx, req)
    if err != nil {
        return "", 0, err
    }

    return resp.Content, resp.TokensUsed, nil
}

// Use it with the compressor
llmClient := &MyLLMClient{provider: claudeProvider}
compressor := conversation.NewContextCompressor(llmClient, logger)
engine := conversation.NewInfiniteContextEngine(kafkaConsumer, compressor, logger)
```

---

## Replaying Conversations

### Full Replay (No Token Limit)

```go
func replayFullConversation(engine *conversation.InfiniteContextEngine, conversationID string) {
    ctx := context.Background()

    messages, err := engine.ReplayConversation(ctx, conversationID)
    if err != nil {
        log.Fatalf("Replay failed: %v", err)
    }

    fmt.Printf("Replayed %d messages\n", len(messages))
    for i, msg := range messages {
        fmt.Printf("  [%d] %s (%s): %s\n", i+1, msg.Role, msg.Model, truncate(msg.Content, 80))
    }
}

func truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen] + "..."
}
```

### Replay with Token-Limited Compression

```go
func replayWithLimit(engine *conversation.InfiniteContextEngine, conversationID string, maxTokens int) {
    ctx := context.Background()

    messages, compressionData, err := engine.ReplayWithCompression(ctx, conversationID, maxTokens)
    if err != nil {
        log.Fatalf("Replay with compression failed: %v", err)
    }

    if compressionData != nil {
        fmt.Printf("Compression applied:\n")
        fmt.Printf("  Strategy: %s\n", compressionData.CompressionType)
        fmt.Printf("  Original: %d messages (%d tokens)\n",
            compressionData.OriginalMessages, compressionData.OriginalTokens)
        fmt.Printf("  Compressed: %d messages (%d tokens)\n",
            compressionData.CompressedMessages, compressionData.CompressedTokens)
        fmt.Printf("  Ratio: %.2f\n", compressionData.CompressionRatio)
        fmt.Printf("  Preserved entities: %v\n", compressionData.PreservedEntities)
        fmt.Printf("  Duration: %dms\n", compressionData.CompressionDuration)
    } else {
        fmt.Println("No compression needed - conversation fits within token limit")
    }

    fmt.Printf("\n%d messages in context:\n", len(messages))
    for _, msg := range messages {
        fmt.Printf("  [%s] %s\n", msg.Role, truncate(msg.Content, 100))
    }
}
```

---

## Getting Conversation Snapshots

```go
func getSnapshot(engine *conversation.InfiniteContextEngine, conversationID string) {
    ctx := context.Background()

    snapshot, err := engine.GetConversationSnapshot(ctx, conversationID)
    if err != nil {
        log.Fatalf("Snapshot failed: %v", err)
    }

    fmt.Printf("Snapshot: %s\n", snapshot.SnapshotID)
    fmt.Printf("  Conversation: %s\n", snapshot.ConversationID)
    fmt.Printf("  Timestamp: %s\n", snapshot.Timestamp.Format(time.RFC3339))
    fmt.Printf("  Messages: %d\n", len(snapshot.Messages))
    fmt.Printf("  Entities: %d\n", len(snapshot.Entities))

    if snapshot.Context != nil {
        fmt.Printf("  Total tokens: %d\n", snapshot.Context.TotalTokens)
        fmt.Printf("  Context usage: %.1f%%\n", snapshot.Context.ContextUsageRatio*100)
    }
}
```

---

## Working with Events

### Creating Conversation Events

```go
func createMessageEvent(conversationID, userID, nodeID string) *conversation.ConversationEvent {
    event := conversation.NewConversationEvent(
        conversation.ConversationEventMessageAdded,
        nodeID,
        conversationID,
        userID,
    )

    event.Message = &conversation.MessageData{
        MessageID: "msg-" + uuid.New().String(),
        Role:      "user",
        Content:   "What is the best approach for API rate limiting?",
        Tokens:    12,
        CreatedAt: time.Now(),
    }

    return event
}
```

### Creating Entity Extraction Events

```go
func createEntityEvent(conversationID, userID, nodeID string) *conversation.ConversationEvent {
    event := conversation.NewConversationEvent(
        conversation.ConversationEventEntityExtracted,
        nodeID,
        conversationID,
        userID,
    )

    event.Entities = []conversation.EntityData{
        {
            EntityID:   "ent-001",
            Type:       "technology",
            Name:       "Redis",
            Value:      "In-memory cache",
            Confidence: 0.95,
            Context:    "Redis was mentioned as the caching layer",
        },
        {
            EntityID:   "ent-002",
            Type:       "concept",
            Name:       "Token Bucket",
            Value:      "Rate limiting algorithm",
            Confidence: 0.88,
            Context:    "Token bucket algorithm for rate limiting",
        },
    }

    return event
}
```

### Creating Debate Round Events

```go
func createDebateRoundEvent(conversationID, userID, nodeID string) *conversation.ConversationEvent {
    event := conversation.NewConversationEvent(
        conversation.ConversationEventDebateRound,
        nodeID,
        conversationID,
        userID,
    )

    event.DebateRound = &conversation.DebateRoundData{
        RoundID:        "round-001",
        RoundNumber:    1,
        Provider:       "claude",
        Model:          "claude-3-opus",
        Role:           "proposer",
        Response:       "I propose using a token bucket algorithm with Redis...",
        TokensUsed:     250,
        ResponseTimeMs: 1200,
        Confidence:     0.92,
        CreatedAt:      time.Now(),
    }

    return event
}
```

### Serializing and Deserializing Events

```go
// Serialize to JSON (for Kafka publishing)
event := createMessageEvent("conv-123", "user-456", "node-1")
jsonData, err := event.ToJSON()
if err != nil {
    log.Fatalf("Serialization failed: %v", err)
}

// Deserialize from JSON (for Kafka consumption)
parsed, err := conversation.ConversationEventFromJSON(jsonData)
if err != nil {
    log.Fatalf("Deserialization failed: %v", err)
}

fmt.Printf("Event: %s (type: %s)\n", parsed.EventID, parsed.EventType)
```

### Cloning Events

```go
// Deep copy an event (useful for modifying without affecting the original)
original := createMessageEvent("conv-123", "user-456", "node-1")
clone := original.Clone()

// Modify the clone without affecting the original
clone.Message.Content = "Modified content"
fmt.Println(original.Message.Content) // Still the original content
```

---

## Using the Context Compressor Directly

### Compress a Set of Messages

```go
func compressMessages(llmClient conversation.LLMClient) {
    compressor := conversation.NewContextCompressor(llmClient, logrus.New())

    messages := []conversation.MessageData{
        {MessageID: "1", Role: "user", Content: "Tell me about Go concurrency"},
        {MessageID: "2", Role: "assistant", Content: "Go provides goroutines and channels..."},
        {MessageID: "3", Role: "user", Content: "How do mutexes work?"},
        {MessageID: "4", Role: "assistant", Content: "sync.Mutex provides mutual exclusion..."},
        // ... many more messages
    }

    entities := []conversation.EntityData{
        {EntityID: "e1", Name: "Go", Type: "language"},
        {EntityID: "e2", Name: "sync.Mutex", Type: "type"},
    }

    maxTokens := 1000

    compressed, data, err := compressor.Compress(context.Background(), messages, entities, maxTokens)
    if err != nil {
        log.Fatalf("Compression failed: %v", err)
    }

    fmt.Printf("Compressed %d -> %d messages (ratio: %.2f)\n",
        data.OriginalMessages, data.CompressedMessages, data.CompressionRatio)
    fmt.Printf("Tokens: %d -> %d\n", data.OriginalTokens, data.CompressedTokens)
}
```

---

## Integration with HelixAgent Debate Service

The ConversationContext module is used in HelixAgent's debate service to maintain conversation history across multi-round debates with multiple LLM providers:

```go
// In the debate service, after each debate round:
func (s *DebateService) recordDebateRound(ctx context.Context, conversationID string, round DebateRound) {
    // 1. Publish the debate round as a conversation event
    event := conversation.NewConversationEvent(
        conversation.ConversationEventDebateRound,
        s.nodeID,
        conversationID,
        "system",
    )
    event.DebateRound = &conversation.DebateRoundData{
        RoundID:        round.ID,
        RoundNumber:    round.Number,
        Provider:       round.Provider,
        Model:          round.Model,
        Role:           round.Role,
        Response:       round.Response,
        TokensUsed:     round.TokensUsed,
        ResponseTimeMs: round.LatencyMs,
        Confidence:     round.Confidence,
        CreatedAt:      time.Now(),
    }

    data, _ := event.ToJSON()
    _ = s.kafkaProducer.Publish(ctx, "conversation-events", conversationID, data)

    // 2. Replay with compression for the next round's context
    messages, _, err := s.contextEngine.ReplayWithCompression(ctx, conversationID, 8000)
    if err != nil {
        log.Printf("Failed to replay context: %v", err)
        return
    }

    // 3. Use compressed context for the next debate round
    s.nextRoundContext = messages
}
```

This pattern ensures that even long-running debates with dozens of rounds maintain full context awareness while fitting within model token limits.
