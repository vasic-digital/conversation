package conversation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConversationEvent(t *testing.T) {
	eventType := ConversationEventMessageAdded
	nodeID := "node-1"
	conversationID := "conv-123"
	userID := "user-456"

	event := NewConversationEvent(eventType, nodeID, conversationID, userID)

	assert.NotNil(t, event)
	assert.Equal(t, eventType, event.EventType)
	assert.Equal(t, nodeID, event.NodeID)
	assert.Equal(t, conversationID, event.ConversationID)
	assert.Equal(t, userID, event.UserID)
	assert.NotEmpty(t, event.EventID)
	assert.True(t, event.EventID[:5] == "cevt-", "EventID should start with cevt- prefix")
	assert.False(t, event.Timestamp.IsZero(), "Timestamp should be set")
	assert.True(t, event.SequenceNumber > 0, "SequenceNumber should be positive")
	// Timestamp should be UTC
	assert.Equal(t, time.UTC, event.Timestamp.Location())
}

func TestConversationEvent_ToJSON(t *testing.T) {
	event := NewConversationEvent(ConversationEventCreated, "node-1", "conv-1", "user-1")
	event.Message = &MessageData{
		MessageID: "msg-1",
		Role:      "user",
		Content:   "Hello world",
		Tokens:    10,
		CreatedAt: time.Now().UTC(),
	}

	data, err := event.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Roundtrip: parse back and verify
	parsed, err := ConversationEventFromJSON(data)
	require.NoError(t, err)
	assert.Equal(t, event.EventID, parsed.EventID)
	assert.Equal(t, event.EventType, parsed.EventType)
	assert.Equal(t, event.ConversationID, parsed.ConversationID)
	assert.Equal(t, event.UserID, parsed.UserID)
	assert.Equal(t, event.NodeID, parsed.NodeID)
	assert.NotNil(t, parsed.Message)
	assert.Equal(t, "msg-1", parsed.Message.MessageID)
	assert.Equal(t, "user", parsed.Message.Role)
	assert.Equal(t, "Hello world", parsed.Message.Content)
	assert.Equal(t, 10, parsed.Message.Tokens)
}

func TestConversationEventFromJSON_Success(t *testing.T) {
	original := &ConversationEvent{
		EventID:        "cevt-test-001",
		EventType:      ConversationEventMessageAdded,
		Timestamp:      time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		NodeID:         "node-a",
		SequenceNumber: 42,
		ConversationID: "conv-abc",
		SessionID:      "session-xyz",
		UserID:         "user-def",
		Entities: []EntityData{
			{EntityID: "ent-1", Type: "person", Name: "Alice", Confidence: 0.95},
		},
		Metadata: map[string]interface{}{"key": "value"},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	parsed, err := ConversationEventFromJSON(data)
	require.NoError(t, err)
	assert.Equal(t, original.EventID, parsed.EventID)
	assert.Equal(t, original.EventType, parsed.EventType)
	assert.Equal(t, original.ConversationID, parsed.ConversationID)
	assert.Equal(t, original.SessionID, parsed.SessionID)
	assert.Equal(t, original.UserID, parsed.UserID)
	assert.Equal(t, original.NodeID, parsed.NodeID)
	assert.Equal(t, original.SequenceNumber, parsed.SequenceNumber)
	assert.Len(t, parsed.Entities, 1)
	assert.Equal(t, "Alice", parsed.Entities[0].Name)
	assert.Equal(t, "value", parsed.Metadata["key"])
}

func TestConversationEventFromJSON_InvalidJSON(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty bytes", []byte{}},
		{"not json", []byte("not json at all")},
		{"truncated json", []byte(`{"event_id": "test"`)},
		{"wrong type", []byte(`{"event_type": 123}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ConversationEventFromJSON(tt.input)
			assert.Error(t, err)
			assert.Nil(t, event)
		})
	}
}

func TestConversationEvent_Clone(t *testing.T) {
	original := NewConversationEvent(ConversationEventMessageAdded, "node-1", "conv-1", "user-1")
	original.Message = &MessageData{
		MessageID: "msg-1",
		Role:      "user",
		Content:   "Hello",
		Tokens:    5,
		CreatedAt: time.Now().UTC(),
	}

	clone := original.Clone()

	// Verify fields match
	assert.Equal(t, original.EventID, clone.EventID)
	assert.Equal(t, original.EventType, clone.EventType)
	assert.Equal(t, original.ConversationID, clone.ConversationID)
	assert.Equal(t, original.UserID, clone.UserID)
	assert.NotNil(t, clone.Message)
	assert.Equal(t, original.Message.Content, clone.Message.Content)

	// Modify clone and verify original is unaffected
	clone.Message.Content = "Modified"
	clone.EventID = "changed"
	assert.Equal(t, "Hello", original.Message.Content)
	assert.NotEqual(t, "changed", original.EventID)
}

func TestConversationEvent_Clone_WithAllFields(t *testing.T) {
	now := time.Now().UTC()
	original := &ConversationEvent{
		EventID:        "cevt-full-001",
		EventType:      ConversationEventDebateRound,
		Timestamp:      now,
		NodeID:         "node-full",
		SequenceNumber: 100,
		ConversationID: "conv-full",
		SessionID:      "sess-full",
		UserID:         "user-full",
		Message: &MessageData{
			MessageID: "msg-full",
			Role:      "assistant",
			Content:   "Full content",
			Model:     "gpt-4",
			Tokens:    50,
			CreatedAt: now,
		},
		Entities: []EntityData{
			{EntityID: "ent-1", Type: "person", Name: "Alice", Value: "engineer", Confidence: 0.9},
			{EntityID: "ent-2", Type: "org", Name: "Acme", Value: "company", Confidence: 0.85},
		},
		Context: &ContextData{
			MessageCount:      10,
			TotalTokens:       5000,
			EntityCount:       2,
			CompressedCount:   3,
			CompressionRatio:  0.4,
			KeyTopics:         []string{"AI", "testing"},
			ActiveEntities:    []string{"Alice", "Acme"},
			ContextWindow:     128000,
			ContextUsageRatio: 0.05,
		},
		Compression: &CompressionData{
			CompressionID:       "comp-1",
			CompressionType:     "hybrid",
			OriginalMessages:    100,
			CompressedMessages:  30,
			OriginalTokens:      50000,
			CompressedTokens:    15000,
			CompressionRatio:    0.3,
			SummaryContent:      "Summary here",
			PreservedEntities:   []string{"Alice", "Acme"},
			LLMModel:            "gpt-4-turbo",
			CompressionDuration: 1500,
			CompressedAt:        now,
		},
		DebateRound: &DebateRoundData{
			RoundID:        "round-1",
			RoundNumber:    1,
			Provider:       "openai",
			Model:          "gpt-4",
			Role:           "proposer",
			Response:       "Debate response",
			TokensUsed:     200,
			ResponseTimeMs: 3000,
			Confidence:     0.92,
			CreatedAt:      now,
		},
		Metadata: map[string]interface{}{
			"source": "test",
			"count":  float64(42),
		},
	}

	clone := original.Clone()

	// Verify all top-level fields
	assert.Equal(t, original.EventID, clone.EventID)
	assert.Equal(t, original.EventType, clone.EventType)
	assert.Equal(t, original.Timestamp, clone.Timestamp)
	assert.Equal(t, original.NodeID, clone.NodeID)
	assert.Equal(t, original.SequenceNumber, clone.SequenceNumber)
	assert.Equal(t, original.ConversationID, clone.ConversationID)
	assert.Equal(t, original.SessionID, clone.SessionID)
	assert.Equal(t, original.UserID, clone.UserID)

	// Verify message deep copy
	require.NotNil(t, clone.Message)
	assert.Equal(t, original.Message.MessageID, clone.Message.MessageID)
	assert.Equal(t, original.Message.Content, clone.Message.Content)
	clone.Message.Content = "changed-msg"
	assert.Equal(t, "Full content", original.Message.Content)

	// Verify entities deep copy
	assert.Len(t, clone.Entities, 2)
	assert.Equal(t, "Alice", clone.Entities[0].Name)

	// Verify context deep copy
	require.NotNil(t, clone.Context)
	assert.Equal(t, original.Context.MessageCount, clone.Context.MessageCount)
	assert.Equal(t, original.Context.TotalTokens, clone.Context.TotalTokens)
	assert.Equal(t, []string{"AI", "testing"}, clone.Context.KeyTopics)
	assert.Equal(t, []string{"Alice", "Acme"}, clone.Context.ActiveEntities)
	clone.Context.KeyTopics[0] = "ML"
	assert.Equal(t, "AI", original.Context.KeyTopics[0])

	// Verify compression deep copy
	require.NotNil(t, clone.Compression)
	assert.Equal(t, original.Compression.CompressionID, clone.Compression.CompressionID)
	assert.Equal(t, original.Compression.CompressionType, clone.Compression.CompressionType)
	assert.Equal(t, original.Compression.OriginalTokens, clone.Compression.OriginalTokens)
	assert.Equal(t, []string{"Alice", "Acme"}, clone.Compression.PreservedEntities)
	clone.Compression.PreservedEntities[0] = "Bob"
	assert.Equal(t, "Alice", original.Compression.PreservedEntities[0])

	// Verify debate round deep copy
	require.NotNil(t, clone.DebateRound)
	assert.Equal(t, original.DebateRound.RoundID, clone.DebateRound.RoundID)
	assert.Equal(t, original.DebateRound.Response, clone.DebateRound.Response)
	clone.DebateRound.Response = "changed-debate"
	assert.Equal(t, "Debate response", original.DebateRound.Response)

	// Verify metadata deep copy
	assert.Equal(t, "test", clone.Metadata["source"])
	clone.Metadata["source"] = "changed"
	assert.Equal(t, "test", original.Metadata["source"])
}

func TestConversationEvent_Clone_NilFields(t *testing.T) {
	original := &ConversationEvent{
		EventID:        "evt-minimal",
		EventType:      ConversationEventCreated,
		ConversationID: "conv-1",
		UserID:         "user-1",
	}

	clone := original.Clone()
	assert.NotNil(t, clone)
	assert.Nil(t, clone.Message)
	assert.Nil(t, clone.Entities)
	assert.Nil(t, clone.Context)
	assert.Nil(t, clone.Compression)
	assert.Nil(t, clone.DebateRound)
	assert.Nil(t, clone.Metadata)
}

func TestConversationEventTypes(t *testing.T) {
	tests := []struct {
		name     string
		constant ConversationEventType
		value    string
	}{
		{"MessageAdded", ConversationEventMessageAdded, "conversation.message.added"},
		{"MessageUpdated", ConversationEventMessageUpdated, "conversation.message.updated"},
		{"MessageDeleted", ConversationEventMessageDeleted, "conversation.message.deleted"},
		{"Created", ConversationEventCreated, "conversation.created"},
		{"Completed", ConversationEventCompleted, "conversation.completed"},
		{"Archived", ConversationEventArchived, "conversation.archived"},
		{"EntityExtracted", ConversationEventEntityExtracted, "conversation.entity.extracted"},
		{"ContextUpdated", ConversationEventContextUpdated, "conversation.context.updated"},
		{"Compressed", ConversationEventCompressed, "conversation.compressed"},
		{"DebateRound", ConversationEventDebateRound, "conversation.debate.round"},
		{"DebateWinner", ConversationEventDebateWinner, "conversation.debate.winner"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, ConversationEventType(tt.value), tt.constant)
		})
	}
}

// Benchmarks

func BenchmarkNewConversationEvent(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewConversationEvent(ConversationEventMessageAdded, "node-1", "conv-1", "user-1")
	}
}

func BenchmarkConversationEventToJSON(b *testing.B) {
	event := NewConversationEvent(ConversationEventMessageAdded, "node-1", "conv-1", "user-1")
	event.Message = &MessageData{
		MessageID: "msg-1",
		Role:      "user",
		Content:   "Hello world",
		Tokens:    10,
		CreatedAt: time.Now().UTC(),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = event.ToJSON()
	}
}

func BenchmarkConversationEventFromJSON(b *testing.B) {
	event := NewConversationEvent(ConversationEventMessageAdded, "node-1", "conv-1", "user-1")
	event.Message = &MessageData{
		MessageID: "msg-1",
		Role:      "user",
		Content:   "Hello world",
		Tokens:    10,
		CreatedAt: time.Now().UTC(),
	}
	data, _ := event.ToJSON()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ConversationEventFromJSON(data)
	}
}

func BenchmarkConversationEventClone(b *testing.B) {
	now := time.Now().UTC()
	event := &ConversationEvent{
		EventID:        "cevt-bench-001",
		EventType:      ConversationEventDebateRound,
		Timestamp:      now,
		NodeID:         "node-bench",
		SequenceNumber: 100,
		ConversationID: "conv-bench",
		SessionID:      "sess-bench",
		UserID:         "user-bench",
		Message: &MessageData{
			MessageID: "msg-bench",
			Role:      "assistant",
			Content:   "Benchmark content for clone test",
			Model:     "gpt-4",
			Tokens:    50,
			CreatedAt: now,
		},
		Entities: []EntityData{
			{EntityID: "ent-1", Type: "person", Name: "Alice", Value: "engineer", Confidence: 0.9},
			{EntityID: "ent-2", Type: "org", Name: "Acme", Value: "company", Confidence: 0.85},
		},
		Metadata: map[string]interface{}{
			"source": "test",
			"count":  float64(42),
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = event.Clone()
	}
}

func TestConversationEvent_ToJSON_AllFields(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	event := &ConversationEvent{
		EventID:        "cevt-json-001",
		EventType:      ConversationEventCompressed,
		Timestamp:      now,
		NodeID:         "node-json",
		SequenceNumber: 99,
		ConversationID: "conv-json",
		SessionID:      "sess-json",
		UserID:         "user-json",
		Compression: &CompressionData{
			CompressionID:    "comp-json",
			CompressionType:  "full",
			OriginalMessages: 50,
		},
		Metadata: map[string]interface{}{"format": "test"},
	}

	data, err := event.ToJSON()
	require.NoError(t, err)

	// Verify JSON structure
	var raw map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	assert.Equal(t, "cevt-json-001", raw["event_id"])
	assert.Equal(t, "conversation.compressed", raw["event_type"])
	assert.Equal(t, "conv-json", raw["conversation_id"])
	assert.Equal(t, "sess-json", raw["session_id"])
	assert.NotNil(t, raw["compression"])
	assert.NotNil(t, raw["metadata"])
}
