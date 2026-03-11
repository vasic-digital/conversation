package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	"digital.vasic.messaging/pkg/broker"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBroker implements broker.MessageBroker for testing
type mockBroker struct{}

func (m *mockBroker) Connect(_ context.Context) error     { return nil }
func (m *mockBroker) Close(_ context.Context) error       { return nil }
func (m *mockBroker) HealthCheck(_ context.Context) error { return nil }
func (m *mockBroker) IsConnected() bool                   { return true }

func (m *mockBroker) Publish(_ context.Context, _ string, _ *broker.Message, _ ...broker.PublishOption) error {
	return nil
}

func (m *mockBroker) PublishBatch(_ context.Context, _ string, _ []*broker.Message, _ ...broker.PublishOption) error {
	return nil
}

func (m *mockBroker) Subscribe(_ context.Context, _ string, _ broker.Handler, _ ...broker.SubscribeOption) (broker.Subscription, error) {
	return nil, nil
}

func (m *mockBroker) Type() broker.BrokerType {
	return broker.BrokerTypeInMemory
}

func (m *mockBroker) Unsubscribe(_ string) error {
	return nil
}

func (m *mockBroker) GetMetrics() *broker.BrokerMetrics {
	return broker.NewBrokerMetrics()
}

// --- ContextCache Tests ---

func TestContextCache_PutAndGet(t *testing.T) {
	cache := &ContextCache{
		cache:   make(map[string]*CachedContext),
		maxSize: 10,
		ttl:     30 * time.Minute,
	}

	cached := &CachedContext{
		ConversationID: "conv-1",
		Messages: []MessageData{
			{MessageID: "m1", Role: "user", Content: "Hello", Tokens: 5},
		},
		Entities: []EntityData{
			{EntityID: "e1", Name: "Alice"},
		},
		Context: &ContextData{
			MessageCount: 1,
			TotalTokens:  5,
		},
		CachedAt: time.Now(),
	}

	cache.Put("conv-1", cached)

	result := cache.Get("conv-1")
	require.NotNil(t, result)
	assert.Equal(t, "conv-1", result.ConversationID)
	assert.Len(t, result.Messages, 1)
	assert.Equal(t, "Hello", result.Messages[0].Content)
	assert.Len(t, result.Entities, 1)
	assert.Equal(t, 1, result.AccessCount)
}

func TestContextCache_Get_Expired(t *testing.T) {
	cache := &ContextCache{
		cache:   make(map[string]*CachedContext),
		maxSize: 10,
		ttl:     1 * time.Millisecond, // Very short TTL
	}

	cached := &CachedContext{
		ConversationID: "conv-expired",
		Messages:       []MessageData{{MessageID: "m1", Content: "old"}},
		CachedAt:       time.Now().Add(-1 * time.Second), // Already expired
	}

	cache.cache["conv-expired"] = cached

	// Should return nil for expired entry
	result := cache.Get("conv-expired")
	assert.Nil(t, result)

	// Entry should be removed from cache
	assert.Equal(t, 0, len(cache.cache))
}

func TestContextCache_Get_NotFound(t *testing.T) {
	cache := &ContextCache{
		cache:   make(map[string]*CachedContext),
		maxSize: 10,
		ttl:     30 * time.Minute,
	}

	result := cache.Get("nonexistent")
	assert.Nil(t, result)
}

func TestContextCache_Eviction(t *testing.T) {
	cache := &ContextCache{
		cache:   make(map[string]*CachedContext),
		maxSize: 3,
		ttl:     30 * time.Minute,
	}

	// Add 3 items with staggered times
	for i := 0; i < 3; i++ {
		cache.Put("conv-"+string(rune('a'+i)), &CachedContext{
			ConversationID: "conv-" + string(rune('a'+i)),
			CachedAt:       time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	assert.Equal(t, 3, cache.Size())

	// Adding a 4th should evict the oldest (conv-a)
	cache.Put("conv-d", &CachedContext{
		ConversationID: "conv-d",
		CachedAt:       time.Now().Add(3 * time.Second),
	})

	assert.Equal(t, 3, cache.Size())

	// conv-a (oldest) should be evicted
	result := cache.Get("conv-a")
	assert.Nil(t, result)

	// conv-d should be present
	result = cache.Get("conv-d")
	assert.NotNil(t, result)
}

func TestContextCache_Clear(t *testing.T) {
	cache := &ContextCache{
		cache:   make(map[string]*CachedContext),
		maxSize: 10,
		ttl:     30 * time.Minute,
	}

	cache.Put("conv-1", &CachedContext{ConversationID: "conv-1", CachedAt: time.Now()})
	cache.Put("conv-2", &CachedContext{ConversationID: "conv-2", CachedAt: time.Now()})
	assert.Equal(t, 2, cache.Size())

	cache.Clear()
	assert.Equal(t, 0, cache.Size())

	// Verify entries are gone
	assert.Nil(t, cache.Get("conv-1"))
	assert.Nil(t, cache.Get("conv-2"))
}

func TestContextCache_Size(t *testing.T) {
	cache := &ContextCache{
		cache:   make(map[string]*CachedContext),
		maxSize: 10,
		ttl:     30 * time.Minute,
	}

	assert.Equal(t, 0, cache.Size())

	cache.Put("conv-1", &CachedContext{ConversationID: "conv-1", CachedAt: time.Now()})
	assert.Equal(t, 1, cache.Size())

	cache.Put("conv-2", &CachedContext{ConversationID: "conv-2", CachedAt: time.Now()})
	assert.Equal(t, 2, cache.Size())

	// Overwrite existing key
	cache.Put("conv-1", &CachedContext{ConversationID: "conv-1", CachedAt: time.Now()})
	assert.Equal(t, 2, cache.Size())
}

// --- InfiniteContextEngine Tests ---

func TestNewInfiniteContextEngine(t *testing.T) {
	broker := &mockBroker{}
	compressor := NewContextCompressor(nil, nil)
	logger := logrus.New()

	engine := NewInfiniteContextEngine(broker, compressor, logger)

	assert.NotNil(t, engine)
	assert.Equal(t, broker, engine.kafkaConsumer)
	assert.Equal(t, compressor, engine.compressor)
	assert.Equal(t, logger, engine.logger)
	assert.NotNil(t, engine.cache)
	assert.Equal(t, 100, engine.cache.maxSize)
	assert.Equal(t, 30*time.Minute, engine.cache.ttl)
}

func TestNewInfiniteContextEngine_NilLogger(t *testing.T) {
	broker := &mockBroker{}
	compressor := NewContextCompressor(nil, nil)

	engine := NewInfiniteContextEngine(broker, compressor, nil)

	assert.NotNil(t, engine)
	assert.NotNil(t, engine.logger)
}

func TestInfiniteContextEngine_ReconstructFromEvents(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	now := time.Now().UTC()
	events := []*ConversationEvent{
		{
			EventType:      ConversationEventMessageAdded,
			SequenceNumber: 2,
			Message: &MessageData{
				MessageID: "msg-2",
				Role:      "assistant",
				Content:   "Hello! How can I help?",
				Tokens:    10,
				CreatedAt: now.Add(1 * time.Minute),
			},
		},
		{
			EventType:      ConversationEventMessageAdded,
			SequenceNumber: 1,
			Message: &MessageData{
				MessageID: "msg-1",
				Role:      "user",
				Content:   "Hi there",
				Tokens:    5,
				CreatedAt: now,
			},
		},
		{
			EventType:      ConversationEventEntityExtracted,
			SequenceNumber: 3,
			Entities: []EntityData{
				{EntityID: "ent-1", Type: "person", Name: "Alice", Confidence: 0.95},
				{EntityID: "ent-2", Type: "org", Name: "Acme", Confidence: 0.88},
			},
		},
		{
			EventType:      ConversationEventCompressed,
			SequenceNumber: 4,
			Compression: &CompressionData{
				CompressionID: "comp-1",
			},
		},
	}

	messages, entities := engine.reconstructFromEvents(events)

	// Should have 2 messages sorted by sequence number
	assert.Len(t, messages, 2)
	assert.Equal(t, "msg-1", messages[0].MessageID)
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, "msg-2", messages[1].MessageID)
	assert.Equal(t, "assistant", messages[1].Role)

	// Should have 2 entities
	assert.Len(t, entities, 2)
	entityNames := make(map[string]bool)
	for _, e := range entities {
		entityNames[e.Name] = true
	}
	assert.True(t, entityNames["Alice"])
	assert.True(t, entityNames["Acme"])
}

func TestInfiniteContextEngine_ReconstructFromEvents_DebateRound(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	now := time.Now().UTC()
	events := []*ConversationEvent{
		{
			EventType:      ConversationEventDebateRound,
			SequenceNumber: 1,
			DebateRound: &DebateRoundData{
				RoundID:    "round-1",
				Model:      "gpt-4",
				Role:       "proposer",
				Response:   "My proposal is...",
				TokensUsed: 50,
				CreatedAt:  now,
			},
		},
	}

	messages, entities := engine.reconstructFromEvents(events)

	assert.Len(t, messages, 1)
	assert.Equal(t, "round-1", messages[0].MessageID)
	assert.Equal(t, "assistant", messages[0].Role)
	assert.Equal(t, "My proposal is...", messages[0].Content)
	assert.Equal(t, "gpt-4", messages[0].Model)
	assert.Equal(t, 50, messages[0].Tokens)
	assert.Empty(t, entities)
}

func TestInfiniteContextEngine_ReconstructFromEvents_Empty(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	messages, entities := engine.reconstructFromEvents([]*ConversationEvent{})

	assert.Empty(t, messages)
	assert.Empty(t, entities)
}

func TestInfiniteContextEngine_ReconstructFromEvents_NilMessage(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	events := []*ConversationEvent{
		{
			EventType:      ConversationEventMessageAdded,
			SequenceNumber: 1,
			Message:        nil, // nil message should be skipped
		},
	}

	messages, entities := engine.reconstructFromEvents(events)
	assert.Empty(t, messages)
	assert.Empty(t, entities)
}

func TestInfiniteContextEngine_ReconstructFromEvents_DeduplicatesEntities(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	events := []*ConversationEvent{
		{
			EventType:      ConversationEventEntityExtracted,
			SequenceNumber: 1,
			Entities: []EntityData{
				{EntityID: "ent-1", Name: "Alice", Confidence: 0.8},
			},
		},
		{
			EventType:      ConversationEventEntityExtracted,
			SequenceNumber: 2,
			Entities: []EntityData{
				{EntityID: "ent-1", Name: "Alice", Confidence: 0.95}, // Same ID, updated
			},
		},
	}

	_, entities := engine.reconstructFromEvents(events)

	// Should deduplicate by EntityID - the later one should overwrite
	assert.Len(t, entities, 1)
	assert.Equal(t, "Alice", entities[0].Name)
	assert.InDelta(t, 0.95, entities[0].Confidence, 0.001)
}

func TestInfiniteContextEngine_CalculateContext(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	messages := []MessageData{
		{Content: "Hello", Tokens: 100},
		{Content: "World", Tokens: 200},
		{Content: "Test", Tokens: 300},
	}
	entities := []EntityData{
		{EntityID: "e1", Name: "Alice"},
		{EntityID: "e2", Name: "Bob"},
	}

	ctx := engine.calculateContext(messages, entities)

	assert.NotNil(t, ctx)
	assert.Equal(t, 3, ctx.MessageCount)
	assert.Equal(t, int64(600), ctx.TotalTokens)
	assert.Equal(t, 2, ctx.EntityCount)
	assert.Equal(t, 128000, ctx.ContextWindow)
	assert.InDelta(t, 600.0/128000.0, ctx.ContextUsageRatio, 0.0001)
}

func TestInfiniteContextEngine_CalculateContext_EmptyMessages(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	ctx := engine.calculateContext([]MessageData{}, []EntityData{})

	assert.NotNil(t, ctx)
	assert.Equal(t, 0, ctx.MessageCount)
	assert.Equal(t, int64(0), ctx.TotalTokens)
	assert.Equal(t, 0, ctx.EntityCount)
	assert.Equal(t, 128000, ctx.ContextWindow)
	assert.InDelta(t, 0.0, ctx.ContextUsageRatio, 0.0001)
}

func TestInfiniteContextEngine_CountTokens(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	tests := []struct {
		name     string
		messages []MessageData
		expected int64
	}{
		{
			name:     "empty",
			messages: []MessageData{},
			expected: 0,
		},
		{
			name: "with explicit tokens",
			messages: []MessageData{
				{Content: "hello", Tokens: 10},
				{Content: "world", Tokens: 20},
			},
			expected: 30,
		},
		{
			name: "estimated tokens",
			messages: []MessageData{
				{Content: "abcdefgh", Tokens: 0},         // 8 / 4 = 2
				{Content: "abcdefghijklmnop", Tokens: 0}, // 16 / 4 = 4
			},
			expected: 6,
		},
		{
			name: "mixed",
			messages: []MessageData{
				{Content: "hi", Tokens: 100},
				{Content: "abcdefghijkl", Tokens: 0}, // 12 / 4 = 3
			},
			expected: 103,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.countTokens(tt.messages)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInfiniteContextEngine_ReplayConversation_Cached(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	// Pre-populate cache
	cachedMessages := []MessageData{
		{MessageID: "m1", Role: "user", Content: "Cached message 1", Tokens: 10},
		{MessageID: "m2", Role: "assistant", Content: "Cached response 1", Tokens: 20},
	}

	engine.cache.Put("conv-cached", &CachedContext{
		ConversationID: "conv-cached",
		Messages:       cachedMessages,
		Entities:       []EntityData{{EntityID: "e1", Name: "TestEntity"}},
		Context: &ContextData{
			MessageCount: 2,
			TotalTokens:  30,
		},
		CachedAt: time.Now(),
	})

	// ReplayConversation should return cached messages without hitting Kafka
	messages, err := engine.ReplayConversation(context.Background(), "conv-cached")
	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Equal(t, "Cached message 1", messages[0].Content)
	assert.Equal(t, "Cached response 1", messages[1].Content)
}

func TestContextCache_PutOverwrite(t *testing.T) {
	cache := &ContextCache{
		cache:   make(map[string]*CachedContext),
		maxSize: 10,
		ttl:     30 * time.Minute,
	}

	cache.Put("conv-1", &CachedContext{
		ConversationID: "conv-1",
		Messages:       []MessageData{{Content: "first"}},
		CachedAt:       time.Now(),
	})

	cache.Put("conv-1", &CachedContext{
		ConversationID: "conv-1",
		Messages:       []MessageData{{Content: "second"}},
		CachedAt:       time.Now(),
	})

	assert.Equal(t, 1, cache.Size())
	result := cache.Get("conv-1")
	require.NotNil(t, result)
	assert.Equal(t, "second", result.Messages[0].Content)
}

func TestContextCache_Get_IncreasesAccessCount(t *testing.T) {
	cache := &ContextCache{
		cache:   make(map[string]*CachedContext),
		maxSize: 10,
		ttl:     30 * time.Minute,
	}

	cache.Put("conv-1", &CachedContext{
		ConversationID: "conv-1",
		CachedAt:       time.Now(),
		AccessCount:    0,
	})

	cache.Get("conv-1")
	cache.Get("conv-1")
	result := cache.Get("conv-1")
	require.NotNil(t, result)
	assert.Equal(t, 3, result.AccessCount)
}

func TestInfiniteContextEngine_ReplayConversation_EmptyEvents(t *testing.T) {
	// When Kafka returns no matching events (e.g., canceled context causes
	// read loop to break immediately), ReplayConversation returns empty slice
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	// Use a context that is already canceled to force fetchConversationEvents
	// to return empty events (the read loop breaks on context cancel)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	messages, err := engine.ReplayConversation(ctx, "conv-empty-events")
	require.NoError(t, err)
	assert.Empty(t, messages)
	assert.NotNil(t, messages) // should be []MessageData{}, not nil
}

func TestInfiniteContextEngine_ReplayWithCompression_NoCompressionNeeded(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	// Pre-populate cache with small conversation that fits within maxTokens
	cachedMessages := []MessageData{
		{MessageID: "m1", Role: "user", Content: "Hello", Tokens: 5},
		{MessageID: "m2", Role: "assistant", Content: "Hi there!", Tokens: 8},
	}
	engine.cache.Put("conv-small", &CachedContext{
		ConversationID: "conv-small",
		Messages:       cachedMessages,
		Entities:       []EntityData{},
		Context: &ContextData{
			MessageCount: 2,
			TotalTokens:  13,
		},
		CachedAt: time.Now(),
	})

	// maxTokens is much larger than total tokens, so no compression needed
	messages, compressionData, err := engine.ReplayWithCompression(
		context.Background(), "conv-small", 10000,
	)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Nil(t, compressionData, "No compression should be needed")
	assert.Equal(t, "Hello", messages[0].Content)
	assert.Equal(t, "Hi there!", messages[1].Content)
}

func TestInfiniteContextEngine_ReplayWithCompression_CompressionNeeded(t *testing.T) {
	mock := &mockLLMClient{response: "Compressed summary.", tokens: 10}
	compressor := NewContextCompressor(mock, nil)
	engine := NewInfiniteContextEngine(&mockBroker{}, compressor, nil)

	// Pre-populate cache with many messages that exceed maxTokens
	cachedMessages := makeMessages(50, "This is a longer message for testing compression", 30)
	entities := []EntityData{
		{EntityID: "e1", Name: "Alice", Value: "engineer", Confidence: 0.9},
	}
	engine.cache.Put("conv-large", &CachedContext{
		ConversationID: "conv-large",
		Messages:       cachedMessages,
		Entities:       entities,
		Context: &ContextData{
			MessageCount: 50,
			TotalTokens:  1500,
		},
		CachedAt: time.Now(),
	})

	// maxTokens is smaller than total tokens, triggering compression
	messages, compressionData, err := engine.ReplayWithCompression(
		context.Background(), "conv-large", 100,
	)
	require.NoError(t, err)
	assert.NotEmpty(t, messages)
	assert.NotNil(t, compressionData, "Compression should have been applied")
	assert.True(t, compressionData.OriginalMessages > 0)
	assert.True(t, compressionData.CompressionRatio > 0)
}

func TestInfiniteContextEngine_ReplayWithCompression_EmptyReplay(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	// Canceled context returns empty events from fetchConversationEvents
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	messages, compressionData, err := engine.ReplayWithCompression(ctx, "conv-empty", 1000)
	require.NoError(t, err)
	assert.Empty(t, messages)
	assert.Nil(t, compressionData) // no compression needed for empty
}

func TestInfiniteContextEngine_ReplayWithCompression_CompressionError(t *testing.T) {
	// LLM client that always fails
	failingMock := &mockLLMClient{
		response: "",
		tokens:   0,
		err:      errors.New("LLM unavailable"),
	}
	compressor := NewContextCompressor(failingMock, nil)
	engine := NewInfiniteContextEngine(&mockBroker{}, compressor, nil)

	// Pre-populate cache with messages that exceed maxTokens
	cachedMessages := makeMessages(50, "Message content for compression error test", 100)
	engine.cache.Put("conv-compress-fail", &CachedContext{
		ConversationID: "conv-compress-fail",
		Messages:       cachedMessages,
		Entities:       []EntityData{},
		Context: &ContextData{
			MessageCount: 50,
			TotalTokens:  5000,
		},
		CachedAt: time.Now(),
	})

	// maxTokens much smaller than total, so compression is needed but will fail
	messages, compressionData, err := engine.ReplayWithCompression(
		context.Background(), "conv-compress-fail", 1,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "compression failed")
	assert.Nil(t, messages)
	assert.Nil(t, compressionData)
}

func TestInfiniteContextEngine_ReplayWithCompression_WithEntitiesFromCache(t *testing.T) {
	mock := &mockLLMClient{response: "Summary with entities.", tokens: 10}
	compressor := NewContextCompressor(mock, nil)
	engine := NewInfiniteContextEngine(&mockBroker{}, compressor, nil)

	// Pre-populate cache with entities
	cachedMessages := makeMessages(30, "Alice and Bob discussed the project", 50)
	entities := []EntityData{
		{EntityID: "e1", Name: "Alice", Value: "engineer", Confidence: 0.95},
		{EntityID: "e2", Name: "Bob", Value: "manager", Confidence: 0.90},
	}
	engine.cache.Put("conv-entities", &CachedContext{
		ConversationID: "conv-entities",
		Messages:       cachedMessages,
		Entities:       entities,
		Context:        &ContextData{MessageCount: 30, TotalTokens: 1500},
		CachedAt:       time.Now(),
	})

	messages, compressionData, err := engine.ReplayWithCompression(
		context.Background(), "conv-entities", 100,
	)
	require.NoError(t, err)
	assert.NotEmpty(t, messages)
	assert.NotNil(t, compressionData)
}

func TestInfiniteContextEngine_GetConversationSnapshot_Success(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	// Pre-populate cache
	cachedMessages := []MessageData{
		{MessageID: "m1", Role: "user", Content: "Hello", Tokens: 5},
		{MessageID: "m2", Role: "assistant", Content: "Hi!", Tokens: 3},
	}
	cachedEntities := []EntityData{
		{EntityID: "e1", Name: "TestUser", Confidence: 0.9},
	}
	contextData := &ContextData{
		MessageCount: 2,
		TotalTokens:  8,
		EntityCount:  1,
	}
	engine.cache.Put("conv-snapshot", &CachedContext{
		ConversationID: "conv-snapshot",
		Messages:       cachedMessages,
		Entities:       cachedEntities,
		Context:        contextData,
		CachedAt:       time.Now(),
	})

	snapshot, err := engine.GetConversationSnapshot(context.Background(), "conv-snapshot")
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.NotEmpty(t, snapshot.SnapshotID)
	assert.Equal(t, "conv-snapshot", snapshot.ConversationID)
	assert.Len(t, snapshot.Messages, 2)
	assert.Len(t, snapshot.Entities, 1)
	assert.NotNil(t, snapshot.Context)
	assert.False(t, snapshot.Timestamp.IsZero())
	assert.Equal(t, "Hello", snapshot.Messages[0].Content)
	assert.Equal(t, "TestUser", snapshot.Entities[0].Name)
}

func TestInfiniteContextEngine_GetConversationSnapshot_NotInCacheAfterReplay(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	// When replay returns empty events, the conversation is not cached
	// (because the empty events path returns early before caching).
	// GetConversationSnapshot should return "not in cache after replay" error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	snapshot, err := engine.GetConversationSnapshot(ctx, "conv-not-cached")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conversation not in cache after replay")
	assert.Nil(t, snapshot)
}

func TestInfiniteContextEngine_ReconstructFromEvents_NilDebateRound(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	events := []*ConversationEvent{
		{
			EventType:      ConversationEventDebateRound,
			SequenceNumber: 1,
			DebateRound:    nil, // nil debate round should be skipped
		},
	}

	messages, entities := engine.reconstructFromEvents(events)
	assert.Empty(t, messages)
	assert.Empty(t, entities)
}

func TestInfiniteContextEngine_ReconstructFromEvents_Ordering(t *testing.T) {
	engine := NewInfiniteContextEngine(&mockBroker{}, NewContextCompressor(nil, nil), nil)

	now := time.Now().UTC()
	// Events arrive out of order but should be sorted by sequence number
	events := []*ConversationEvent{
		{
			EventType:      ConversationEventMessageAdded,
			SequenceNumber: 3,
			Message:        &MessageData{MessageID: "msg-3", Role: "assistant", Content: "Third", CreatedAt: now.Add(2 * time.Minute)},
		},
		{
			EventType:      ConversationEventMessageAdded,
			SequenceNumber: 1,
			Message:        &MessageData{MessageID: "msg-1", Role: "user", Content: "First", CreatedAt: now},
		},
		{
			EventType:      ConversationEventMessageAdded,
			SequenceNumber: 2,
			Message:        &MessageData{MessageID: "msg-2", Role: "assistant", Content: "Second", CreatedAt: now.Add(1 * time.Minute)},
		},
	}

	messages, _ := engine.reconstructFromEvents(events)

	require.Len(t, messages, 3)
	assert.Equal(t, "First", messages[0].Content)
	assert.Equal(t, "Second", messages[1].Content)
	assert.Equal(t, "Third", messages[2].Content)
}
