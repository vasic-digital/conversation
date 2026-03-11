package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLLMClient implements LLMClient for testing
type mockLLMClient struct {
	response string
	tokens   int
	err      error
	calls    int
}

func (m *mockLLMClient) Complete(_ context.Context, _ string, _ int) (string, int, error) {
	m.calls++
	return m.response, m.tokens, m.err
}

func TestDefaultCompressionConfig(t *testing.T) {
	config := DefaultCompressionConfig()

	assert.NotNil(t, config)
	assert.Equal(t, CompressionStrategyHybrid, config.Strategy)
	assert.Equal(t, 10, config.WindowSize)
	assert.InDelta(t, 0.3, config.TargetRatio, 0.001)
	assert.True(t, config.PreserveEntities)
	assert.True(t, config.PreserveTopics)
	assert.Equal(t, "gpt-4-turbo", config.LLMModel)
}

func TestNewContextCompressor_NilLogger(t *testing.T) {
	mock := &mockLLMClient{response: "summary", tokens: 10}
	cc := NewContextCompressor(mock, nil)

	assert.NotNil(t, cc)
	assert.NotNil(t, cc.logger)
	assert.Equal(t, mock, cc.llmClient)
}

func TestNewContextCompressor_WithLogger(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	mock := &mockLLMClient{response: "summary", tokens: 10}

	cc := NewContextCompressor(mock, logger)

	assert.NotNil(t, cc)
	assert.Equal(t, logger, cc.logger)
	assert.Equal(t, mock, cc.llmClient)
}

func makeMessages(count int, content string, tokens int) []MessageData {
	msgs := make([]MessageData, count)
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = MessageData{
			MessageID: "msg-" + string(rune('a'+i%26)),
			Role:      role,
			Content:   content,
			Tokens:    tokens,
			CreatedAt: now.Add(time.Duration(i) * time.Minute),
		}
	}
	return msgs
}

func TestContextCompressor_Compress_WindowSummary(t *testing.T) {
	mock := &mockLLMClient{response: "Window summary of messages.", tokens: 15}
	cc := NewContextCompressor(mock, nil)

	// Create enough messages so there are older messages to summarize
	messages := makeMessages(60, "Test message content here", 20)
	entities := []EntityData{}

	// Temporarily override default config strategy by testing through the Compress method
	// The Compress method always uses DefaultCompressionConfig which is hybrid.
	// We test window summary indirectly through hybrid (which calls entity graph then window).
	// For direct testing, we call the private method via the public Compress.
	// Since Compress always uses hybrid, we test the hybrid path which incorporates window summary.

	compressed, compressionData, err := cc.Compress(context.Background(), messages, entities, 500)
	require.NoError(t, err)
	assert.NotNil(t, compressionData)
	assert.NotEmpty(t, compressed)
	assert.Equal(t, "hybrid", compressionData.CompressionType)
	assert.Equal(t, 60, compressionData.OriginalMessages)
	assert.True(t, compressionData.CompressedMessages <= 60)
	assert.True(t, compressionData.OriginalTokens > 0)
	assert.NotEmpty(t, compressionData.CompressionID)
	assert.Equal(t, "gpt-4-turbo", compressionData.LLMModel)
	assert.True(t, compressionData.CompressionDuration >= 0)
	assert.False(t, compressionData.CompressedAt.IsZero())
}

func TestContextCompressor_Compress_EntityGraph(t *testing.T) {
	mock := &mockLLMClient{response: "Entity-aware summary.", tokens: 12}
	cc := NewContextCompressor(mock, nil)

	messages := makeMessages(30, "Alice and Bob discussed the project at Acme Corp", 25)
	entities := []EntityData{
		{EntityID: "e1", Type: "person", Name: "Alice", Value: "engineer", Confidence: 0.95},
		{EntityID: "e2", Type: "person", Name: "Bob", Value: "manager", Confidence: 0.90},
		{EntityID: "e3", Type: "org", Name: "Acme Corp", Value: "company", Confidence: 0.88},
	}

	// The default strategy is hybrid, which starts with entity graph
	compressed, compressionData, err := cc.Compress(context.Background(), messages, entities, 300)
	require.NoError(t, err)
	assert.NotNil(t, compressionData)
	assert.NotEmpty(t, compressed)
	assert.True(t, len(compressionData.PreservedEntities) > 0)
}

func TestContextCompressor_Compress_Full(t *testing.T) {
	mock := &mockLLMClient{response: "Full conversation summary covering all main points.", tokens: 30}
	cc := NewContextCompressor(mock, nil)

	// Create many messages with high token counts to force full compression in hybrid
	messages := makeMessages(100, "Long message content that uses many tokens for testing purposes", 100)
	entities := []EntityData{
		{EntityID: "e1", Type: "topic", Name: "testing", Confidence: 0.9},
	}

	// Use a very small maxTokens to force hybrid all the way to full compression
	compressed, compressionData, err := cc.Compress(context.Background(), messages, entities, 1)
	require.NoError(t, err)
	assert.NotNil(t, compressionData)
	assert.NotEmpty(t, compressed)
}

func TestContextCompressor_Compress_Hybrid(t *testing.T) {
	mock := &mockLLMClient{response: "Hybrid summary result.", tokens: 15}
	cc := NewContextCompressor(mock, nil)

	messages := makeMessages(40, "Content for hybrid compression testing", 20)
	entities := []EntityData{
		{EntityID: "e1", Type: "person", Name: "Content", Confidence: 0.85},
	}

	compressed, compressionData, err := cc.Compress(context.Background(), messages, entities, 400)
	require.NoError(t, err)
	assert.NotNil(t, compressionData)
	assert.NotEmpty(t, compressed)
	assert.Equal(t, "hybrid", compressionData.CompressionType)
	assert.True(t, compressionData.CompressionRatio > 0)
}

func TestContextCompressor_Compress_NilLLMClient(t *testing.T) {
	// When llmClient is nil, summarization falls back to simple concatenation
	cc := NewContextCompressor(nil, nil)

	messages := makeMessages(30, "Some message content", 20)
	entities := []EntityData{}

	compressed, compressionData, err := cc.Compress(context.Background(), messages, entities, 200)
	require.NoError(t, err)
	assert.NotNil(t, compressionData)
	assert.NotEmpty(t, compressed)

	// Check that fallback summaries are used (contain "[Summary of")
	foundSummary := false
	for _, msg := range compressed {
		if msg.Role == "system" && len(msg.Content) > 0 {
			foundSummary = true
			break
		}
	}
	// With enough messages, some should be summarized
	if len(messages) > 20 {
		assert.True(t, foundSummary, "Expected fallback summary messages when llmClient is nil")
	}
}

func TestContextCompressor_Compress_LLMError(t *testing.T) {
	mock := &mockLLMClient{
		response: "",
		tokens:   0,
		err:      errors.New("LLM service unavailable"),
	}
	cc := NewContextCompressor(mock, nil)

	messages := makeMessages(30, "Test content for error handling", 20)
	entities := []EntityData{}

	// The hybrid strategy tries entity graph -> window summary -> full compression.
	// Entity graph and window summary fall back on LLM error (keeping originals),
	// but if tokens still exceed maxTokens, full compression is attempted,
	// and summarizeConversation propagates the LLM error.
	// With 30 messages * 20 tokens = 600 tokens and maxTokens=200,
	// the final full compression will fail.
	_, _, err := cc.Compress(context.Background(), messages, entities, 200)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM service unavailable")
}

func TestContextCompressor_Compress_LLMError_FallbackSuccess(t *testing.T) {
	mock := &mockLLMClient{
		response: "",
		tokens:   0,
		err:      errors.New("LLM service unavailable"),
	}
	cc := NewContextCompressor(mock, nil)

	// Use messages where total tokens fit within maxTokens after entity graph fallback
	// so full compression is never reached
	messages := makeMessages(10, "Short msg", 5)
	entities := []EntityData{}

	// 10 messages * 5 tokens = 50, maxTokens=10000 => no compression needed after entity graph
	compressed, compressionData, err := cc.Compress(context.Background(), messages, entities, 10000)
	require.NoError(t, err)
	assert.NotNil(t, compressionData)
	assert.NotEmpty(t, compressed)
}

func TestContextCompressor_CountTokens(t *testing.T) {
	cc := NewContextCompressor(nil, nil)

	tests := []struct {
		name     string
		messages []MessageData
		expected int64
	}{
		{
			name:     "empty messages",
			messages: []MessageData{},
			expected: 0,
		},
		{
			name: "messages with explicit tokens",
			messages: []MessageData{
				{Content: "hello", Tokens: 10},
				{Content: "world", Tokens: 20},
			},
			expected: 30,
		},
		{
			name: "messages without tokens estimated by length",
			messages: []MessageData{
				{Content: "abcdefgh", Tokens: 0},         // 8 chars / 4 = 2
				{Content: "abcdefghijklmnop", Tokens: 0}, // 16 chars / 4 = 4
			},
			expected: 6,
		},
		{
			name: "mixed tokens and estimation",
			messages: []MessageData{
				{Content: "hello", Tokens: 50},
				{Content: "abcdefghijkl", Tokens: 0}, // 12 / 4 = 3
			},
			expected: 53,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cc.countTokens(tt.messages)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContextCompressor_HasEntities(t *testing.T) {
	cc := NewContextCompressor(nil, nil)

	entities := []EntityData{
		{EntityID: "e1", Name: "Alice", Value: "engineer"},
		{EntityID: "e2", Name: "Kubernetes", Value: "platform"},
	}

	tests := []struct {
		name     string
		msg      MessageData
		expected bool
	}{
		{
			name:     "message contains entity name",
			msg:      MessageData{Content: "Alice went to the store"},
			expected: true,
		},
		{
			name:     "message contains entity name case insensitive",
			msg:      MessageData{Content: "ALICE is great"},
			expected: true,
		},
		{
			name:     "message contains entity value",
			msg:      MessageData{Content: "She is an engineer"},
			expected: true,
		},
		{
			name:     "message contains no entities",
			msg:      MessageData{Content: "The weather is nice today"},
			expected: false,
		},
		{
			name:     "empty message",
			msg:      MessageData{Content: ""},
			expected: false,
		},
		{
			name:     "message contains second entity",
			msg:      MessageData{Content: "Deploy to kubernetes cluster"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cc.hasEntities(tt.msg, entities)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContextCompressor_HasEntities_EmptyEntities(t *testing.T) {
	cc := NewContextCompressor(nil, nil)
	msg := MessageData{Content: "Some content"}
	assert.False(t, cc.hasEntities(msg, []EntityData{}))
	assert.False(t, cc.hasEntities(msg, nil))
}

func TestContextCompressor_ExtractEntityNames(t *testing.T) {
	cc := NewContextCompressor(nil, nil)

	entities := []EntityData{
		{EntityID: "e1", Name: "Alice", Value: "engineer"},
		{EntityID: "e2", Name: "Bob", Value: "manager"},
		{EntityID: "e3", Name: "Acme", Value: "company"},
	}

	messages := []MessageData{
		{Content: "Alice talked to Bob about the project"},
		{Content: "The deadline is tomorrow"},
		{Content: "Acme Corp will sponsor the event"},
	}

	names := cc.extractEntityNames(messages, entities)

	// Should find Alice, Bob, and Acme
	assert.Len(t, names, 3)
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	assert.True(t, nameSet["Alice"])
	assert.True(t, nameSet["Bob"])
	assert.True(t, nameSet["Acme"])
}

func TestContextCompressor_ExtractEntityNames_NoMatches(t *testing.T) {
	cc := NewContextCompressor(nil, nil)

	entities := []EntityData{
		{EntityID: "e1", Name: "Charlie", Value: "developer"},
	}

	messages := []MessageData{
		{Content: "No entities mentioned here"},
	}

	names := cc.extractEntityNames(messages, entities)
	assert.Empty(t, names)
}

func TestContextCompressor_ExtractEntityNames_EmptyInputs(t *testing.T) {
	cc := NewContextCompressor(nil, nil)

	names := cc.extractEntityNames([]MessageData{}, []EntityData{})
	assert.Empty(t, names)
}

func TestCompressionStrategies_Constants(t *testing.T) {
	assert.Equal(t, CompressionStrategy("window_summary"), CompressionStrategyWindowSummary)
	assert.Equal(t, CompressionStrategy("entity_graph"), CompressionStrategyEntityGraph)
	assert.Equal(t, CompressionStrategy("full"), CompressionStrategyFull)
	assert.Equal(t, CompressionStrategy("hybrid"), CompressionStrategyHybrid)
}

func TestContextCompressor_Compress_CompressionDataFields(t *testing.T) {
	mock := &mockLLMClient{response: "Summary text.", tokens: 10}
	cc := NewContextCompressor(mock, nil)

	now := time.Now()
	messages := makeMessages(25, "Content for testing compression data fields", 15)
	entities := []EntityData{
		{EntityID: "e1", Name: "Content", Confidence: 0.9},
	}

	compressed, data, err := cc.Compress(context.Background(), messages, entities, 200)
	require.NoError(t, err)
	require.NotNil(t, data)
	require.NotEmpty(t, compressed)

	// Verify CompressionData fields
	assert.NotEmpty(t, data.CompressionID)
	assert.Equal(t, "hybrid", data.CompressionType)
	assert.Equal(t, 25, data.OriginalMessages)
	assert.True(t, data.CompressedMessages > 0)
	assert.True(t, data.OriginalTokens > 0)
	assert.True(t, data.CompressedTokens >= 0)
	assert.True(t, data.CompressionRatio > 0)
	assert.Equal(t, "gpt-4-turbo", data.LLMModel)
	assert.True(t, data.CompressionDuration >= 0)
	assert.True(t, data.CompressedAt.After(now) || data.CompressedAt.Equal(now))
}

func TestContextCompressor_SummarizeWindow_EmptyWindow(t *testing.T) {
	cc := NewContextCompressor(nil, nil)
	summary, tokens, err := cc.summarizeWindow(context.Background(), []MessageData{}, nil)
	require.NoError(t, err)
	assert.Equal(t, "", summary)
	assert.Equal(t, 0, tokens)
}

func TestContextCompressor_SummarizeWindow_NilLLMClient(t *testing.T) {
	cc := NewContextCompressor(nil, nil)

	window := []MessageData{
		{MessageID: "m1", Role: "user", Content: "Hello there", Tokens: 5},
		{MessageID: "m2", Role: "assistant", Content: "Hi!", Tokens: 3},
	}

	summary, tokens, err := cc.summarizeWindow(context.Background(), window, nil)
	require.NoError(t, err)
	assert.Contains(t, summary, "[Summary of 2 messages]")
	assert.Equal(t, 10, tokens)
}

func TestContextCompressor_SummarizeWindow_LLMError(t *testing.T) {
	mock := &mockLLMClient{
		response: "",
		tokens:   0,
		err:      errors.New("LLM service down"),
	}
	cc := NewContextCompressor(mock, nil)

	window := []MessageData{
		{MessageID: "m1", Role: "user", Content: "Test message", Tokens: 5},
	}

	summary, tokens, err := cc.summarizeWindow(context.Background(), window, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM summarization failed")
	assert.Equal(t, "", summary)
	assert.Equal(t, 0, tokens)
}

func TestContextCompressor_SummarizeWindow_WithEntities(t *testing.T) {
	mock := &mockLLMClient{response: "Summary mentioning Alice.", tokens: 15}
	cc := NewContextCompressor(mock, nil)

	window := []MessageData{
		{MessageID: "m1", Role: "user", Content: "Alice asked about the project", Tokens: 10},
	}
	entities := []EntityData{
		{EntityID: "e1", Name: "Alice", Value: "engineer", Confidence: 0.9},
	}

	summary, tokens, err := cc.summarizeWindow(context.Background(), window, entities)
	require.NoError(t, err)
	assert.Equal(t, "Summary mentioning Alice.", summary)
	assert.Equal(t, 15, tokens)
	assert.Equal(t, 1, mock.calls)
}

func TestContextCompressor_SummarizeConversation_NilLLMClient(t *testing.T) {
	cc := NewContextCompressor(nil, nil)

	messages := makeMessages(10, "Test message", 5)
	entities := []EntityData{
		{EntityID: "e1", Name: "Test", Confidence: 0.8},
	}

	summary, tokens, err := cc.summarizeConversation(context.Background(), messages, entities)
	require.NoError(t, err)
	assert.Contains(t, summary, "[Summary of 10 messages with 1 entities]")
	assert.Equal(t, 20, tokens)
}

func TestContextCompressor_SummarizeConversation_LLMError(t *testing.T) {
	mock := &mockLLMClient{
		response: "",
		tokens:   0,
		err:      errors.New("LLM timeout"),
	}
	cc := NewContextCompressor(mock, nil)

	messages := makeMessages(5, "Message content", 10)
	summary, tokens, err := cc.summarizeConversation(context.Background(), messages, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM summarization failed")
	assert.Equal(t, "", summary)
	assert.Equal(t, 0, tokens)
}

func TestContextCompressor_SummarizeConversation_WithEntities(t *testing.T) {
	mock := &mockLLMClient{response: "Full summary of conversation.", tokens: 25}
	cc := NewContextCompressor(mock, nil)

	messages := []MessageData{
		{MessageID: "m1", Role: "user", Content: "Alice needs a report from Bob", Tokens: 10},
		{MessageID: "m2", Role: "assistant", Content: "I will prepare that for Alice", Tokens: 12},
	}
	entities := []EntityData{
		{EntityID: "e1", Name: "Alice", Value: "analyst"},
		{EntityID: "e2", Name: "Bob", Value: "manager"},
	}

	summary, tokens, err := cc.summarizeConversation(context.Background(), messages, entities)
	require.NoError(t, err)
	assert.Equal(t, "Full summary of conversation.", summary)
	assert.Equal(t, 25, tokens)
}

func TestContextCompressor_CompressWindowSummary_Direct(t *testing.T) {
	mock := &mockLLMClient{response: "Window summary.", tokens: 8}
	cc := NewContextCompressor(mock, nil)

	messages := makeMessages(30, "Window summary test content", 15)
	entities := []EntityData{}
	config := DefaultCompressionConfig()
	config.Strategy = CompressionStrategyWindowSummary

	compressed, err := cc.compressWindowSummary(context.Background(), messages, entities, config)
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)
	// Recent messages should be preserved, older messages summarized
	assert.True(t, len(compressed) < len(messages))
}

func TestContextCompressor_CompressWindowSummary_LLMError(t *testing.T) {
	mock := &mockLLMClient{
		response: "",
		tokens:   0,
		err:      errors.New("LLM error"),
	}
	cc := NewContextCompressor(mock, nil)

	messages := makeMessages(30, "Content for fallback test", 15)
	entities := []EntityData{}
	config := DefaultCompressionConfig()

	// When LLM fails, compressWindowSummary keeps original messages
	compressed, err := cc.compressWindowSummary(context.Background(), messages, entities, config)
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)
	// Should have kept all messages since LLM failed (originals kept)
	assert.Len(t, compressed, len(messages))
}

func TestContextCompressor_CompressEntityGraph_Direct(t *testing.T) {
	mock := &mockLLMClient{response: "Entity summary.", tokens: 8}
	cc := NewContextCompressor(mock, nil)

	messages := []MessageData{
		{MessageID: "m1", Role: "user", Content: "Alice discussed the plan", Tokens: 10, CreatedAt: time.Now()},
		{MessageID: "m2", Role: "assistant", Content: "No entities here", Tokens: 8, CreatedAt: time.Now()},
		{MessageID: "m3", Role: "user", Content: "Bob joined the call", Tokens: 10, CreatedAt: time.Now()},
		{MessageID: "m4", Role: "assistant", Content: "Generic response one", Tokens: 8, CreatedAt: time.Now()},
		{MessageID: "m5", Role: "user", Content: "Generic question two", Tokens: 8, CreatedAt: time.Now()},
		{MessageID: "m6", Role: "assistant", Content: "Generic response two", Tokens: 8, CreatedAt: time.Now()},
		{MessageID: "m7", Role: "user", Content: "Generic question three", Tokens: 8, CreatedAt: time.Now()},
		{MessageID: "m8", Role: "assistant", Content: "Generic response three", Tokens: 8, CreatedAt: time.Now()},
		{MessageID: "m9", Role: "user", Content: "Generic question four", Tokens: 8, CreatedAt: time.Now()},
		{MessageID: "m10", Role: "assistant", Content: "Generic response four", Tokens: 8, CreatedAt: time.Now()},
		{MessageID: "m11", Role: "user", Content: "Generic question five", Tokens: 8, CreatedAt: time.Now()},
		{MessageID: "m12", Role: "assistant", Content: "Alice came back", Tokens: 8, CreatedAt: time.Now()},
	}
	entities := []EntityData{
		{EntityID: "e1", Name: "Alice", Value: "engineer", Confidence: 0.95},
		{EntityID: "e2", Name: "Bob", Value: "manager", Confidence: 0.90},
	}
	config := DefaultCompressionConfig()

	compressed, err := cc.compressEntityGraph(context.Background(), messages, entities, config)
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)
}

func TestContextCompressor_CompressFull_Direct(t *testing.T) {
	mock := &mockLLMClient{response: "Full conversation summary.", tokens: 20}
	cc := NewContextCompressor(mock, nil)

	messages := makeMessages(25, "Content for full compression", 15)
	entities := []EntityData{
		{EntityID: "e1", Name: "Content", Confidence: 0.85},
	}
	config := DefaultCompressionConfig()

	compressed, err := cc.compressFull(context.Background(), messages, entities, config)
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)
	// Should have the summary as first message plus recent messages
	assert.Equal(t, "system", compressed[0].Role)
	assert.Equal(t, "Full conversation summary.", compressed[0].Content)
	assert.Equal(t, "full-summary", compressed[0].MessageID)
}

func TestContextCompressor_CompressFull_LLMError(t *testing.T) {
	mock := &mockLLMClient{
		response: "",
		tokens:   0,
		err:      errors.New("LLM unavailable"),
	}
	cc := NewContextCompressor(mock, nil)

	messages := makeMessages(20, "Content for error test", 15)
	entities := []EntityData{}
	config := DefaultCompressionConfig()

	compressed, err := cc.compressFull(context.Background(), messages, entities, config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM summarization failed")
	assert.Nil(t, compressed)
}

func TestContextCompressor_CompressHybrid_TokensFitAfterEntityGraph(t *testing.T) {
	mock := &mockLLMClient{response: "Compact summary.", tokens: 5}
	cc := NewContextCompressor(mock, nil)

	// Small set of messages that will fit after entity graph compression
	messages := makeMessages(5, "Short", 3)
	entities := []EntityData{}
	config := DefaultCompressionConfig()

	// maxTokens is large enough that entity graph result fits
	compressed, err := cc.compressHybrid(context.Background(), messages, entities, 10000, config)
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)
}

func TestContextCompressor_CompressHybrid_NeedsWindowSummary(t *testing.T) {
	mock := &mockLLMClient{response: "Summary.", tokens: 5}
	cc := NewContextCompressor(mock, nil)

	// Enough messages to exceed a moderate token limit after entity graph
	messages := makeMessages(40, "Moderate length message content here", 20)
	entities := []EntityData{}
	config := DefaultCompressionConfig()

	// maxTokens forces window summary after entity graph
	compressed, err := cc.compressHybrid(context.Background(), messages, entities, 200, config)
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)
}

func TestContextCompressor_CompressHybrid_NeedsFull(t *testing.T) {
	mock := &mockLLMClient{response: "Tiny summary.", tokens: 3}
	cc := NewContextCompressor(mock, nil)

	// Many messages with high token counts
	messages := makeMessages(50, "Long message content for full compression path", 100)
	entities := []EntityData{}
	config := DefaultCompressionConfig()

	// Very small maxTokens forces all the way to full compression
	compressed, err := cc.compressHybrid(context.Background(), messages, entities, 1, config)
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)
}

func TestContextCompressor_ExtractPreservedEntities(t *testing.T) {
	cc := NewContextCompressor(nil, nil)

	messages := []MessageData{
		{Content: "Alice and Bob met at the office"},
	}
	entities := []EntityData{
		{EntityID: "e1", Name: "Alice", Value: "engineer"},
		{EntityID: "e2", Name: "Bob", Value: "manager"},
		{EntityID: "e3", Name: "Charlie", Value: "developer"},
	}

	preserved := cc.extractPreservedEntities(messages, entities)
	assert.Len(t, preserved, 2)
	nameSet := make(map[string]bool)
	for _, n := range preserved {
		nameSet[n] = true
	}
	assert.True(t, nameSet["Alice"])
	assert.True(t, nameSet["Bob"])
	assert.False(t, nameSet["Charlie"])
}

func TestContextCompressor_Min(t *testing.T) {
	assert.Equal(t, 3, min(3, 5))
	assert.Equal(t, 3, min(5, 3))
	assert.Equal(t, 0, min(0, 0))
	assert.Equal(t, -1, min(-1, 5))
}

func TestContextCompressor_Compress_SmallMessageSet(t *testing.T) {
	mock := &mockLLMClient{response: "Small summary.", tokens: 5}
	cc := NewContextCompressor(mock, nil)

	messages := []MessageData{
		{MessageID: "m1", Role: "user", Content: "Hi", Tokens: 1, CreatedAt: time.Now()},
		{MessageID: "m2", Role: "assistant", Content: "Hello!", Tokens: 2, CreatedAt: time.Now()},
	}

	compressed, data, err := cc.Compress(context.Background(), messages, nil, 100)
	require.NoError(t, err)
	assert.NotNil(t, data)
	assert.NotEmpty(t, compressed)
}
