package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"digital.vasic.messaging/pkg/broker"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
)

// InfiniteContextEngine provides unlimited context through Kafka event replay
type InfiniteContextEngine struct {
	kafkaConsumer broker.MessageBroker
	compressor    *ContextCompressor
	cache         *ContextCache
	logger        *logrus.Logger
	mu            sync.RWMutex
}

// ContextCache provides LRU caching for replayed conversations
type ContextCache struct {
	cache   map[string]*CachedContext
	maxSize int
	ttl     time.Duration
	mu      sync.RWMutex
}

// CachedContext represents a cached conversation context
type CachedContext struct {
	ConversationID string
	Messages       []MessageData
	Entities       []EntityData
	Context        *ContextData
	CachedAt       time.Time
	AccessCount    int
}

// NewInfiniteContextEngine creates a new infinite context engine
func NewInfiniteContextEngine(
	kafkaConsumer broker.MessageBroker,
	compressor *ContextCompressor,
	logger *logrus.Logger,
) *InfiniteContextEngine {
	if logger == nil {
		logger = logrus.New()
	}

	return &InfiniteContextEngine{
		kafkaConsumer: kafkaConsumer,
		compressor:    compressor,
		cache: &ContextCache{
			cache:   make(map[string]*CachedContext),
			maxSize: 100,
			ttl:     30 * time.Minute,
		},
		logger: logger,
	}
}

// ReplayConversation replays an entire conversation from Kafka
// No token limit - full history preserved in event stream
func (ice *InfiniteContextEngine) ReplayConversation(ctx context.Context, conversationID string) ([]MessageData, error) {
	ice.mu.Lock()
	defer ice.mu.Unlock()

	ice.logger.WithField("conversation_id", conversationID).Info("Replaying conversation from Kafka")

	// Check cache first
	if cached := ice.cache.Get(conversationID); cached != nil {
		ice.logger.Debug("Conversation found in cache")
		return cached.Messages, nil
	}

	// Replay from Kafka event stream
	events, err := ice.fetchConversationEvents(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch conversation events: %w", err)
	}

	if len(events) == 0 {
		return []MessageData{}, nil
	}

	// Reconstruct conversation from events
	messages, entities := ice.reconstructFromEvents(events)

	// Calculate context data
	contextData := ice.calculateContext(messages, entities)

	// Cache the replayed conversation
	ice.cache.Put(conversationID, &CachedContext{
		ConversationID: conversationID,
		Messages:       messages,
		Entities:       entities,
		Context:        contextData,
		CachedAt:       time.Now(),
	})

	ice.logger.WithFields(logrus.Fields{
		"conversation_id": conversationID,
		"message_count":   len(messages),
		"entity_count":    len(entities),
		"total_tokens":    contextData.TotalTokens,
	}).Info("Conversation replayed successfully")

	return messages, nil
}

// ReplayWithCompression replays conversation and compresses if needed
func (ice *InfiniteContextEngine) ReplayWithCompression(
	ctx context.Context,
	conversationID string,
	maxTokens int,
) ([]MessageData, *CompressionData, error) {
	// Replay full conversation
	messages, err := ice.ReplayConversation(ctx, conversationID)
	if err != nil {
		return nil, nil, err
	}

	// Check if compression needed
	totalTokens := ice.countTokens(messages)
	if totalTokens <= int64(maxTokens) {
		// No compression needed
		return messages, nil, nil
	}

	ice.logger.WithFields(logrus.Fields{
		"conversation_id": conversationID,
		"total_tokens":    totalTokens,
		"max_tokens":      maxTokens,
	}).Info("Compression required")

	// Get cached entities
	cached := ice.cache.Get(conversationID)
	var entities []EntityData
	if cached != nil {
		entities = cached.Entities
	}

	// Compress conversation
	compressed, compressionData, err := ice.compressor.Compress(ctx, messages, entities, maxTokens)
	if err != nil {
		return nil, nil, fmt.Errorf("compression failed: %w", err)
	}

	ice.logger.WithFields(logrus.Fields{
		"conversation_id":     conversationID,
		"original_messages":   len(messages),
		"compressed_messages": len(compressed),
		"compression_ratio":   compressionData.CompressionRatio,
	}).Info("Conversation compressed successfully")

	return compressed, compressionData, nil
}

// GetConversationSnapshot creates a snapshot of conversation state
func (ice *InfiniteContextEngine) GetConversationSnapshot(ctx context.Context, conversationID string) (*ConversationSnapshot, error) {
	messages, err := ice.ReplayConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	cached := ice.cache.Get(conversationID)
	if cached == nil {
		return nil, fmt.Errorf("conversation not in cache after replay")
	}

	snapshot := &ConversationSnapshot{
		SnapshotID:     uuid.New().String(),
		ConversationID: conversationID,
		Timestamp:      time.Now(),
		Messages:       messages,
		Entities:       cached.Entities,
		Context:        cached.Context,
	}

	return snapshot, nil
}

// fetchConversationEvents fetches all events for a conversation from Kafka
func (ice *InfiniteContextEngine) fetchConversationEvents(ctx context.Context, conversationID string) ([]*ConversationEvent, error) {
	ice.logger.WithField("conversation_id", conversationID).Debug("Fetching events from Kafka")

	// Kafka configuration
	// In production, these would come from configuration
	brokers := []string{"localhost:9092"}
	topic := "conversation-events"
	groupID := fmt.Sprintf("infinite-context-%s", conversationID)

	// Create Kafka reader
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   brokers,
		Topic:     topic,
		GroupID:   groupID,
		Partition: 0, // Read from partition 0 - would need to handle multiple partitions
		MinBytes:  1,
		MaxBytes:  10e6, // 10MB
	})
	defer func() { _ = reader.Close() }()

	// Seek to beginning to read all events
	if err := reader.SetOffset(kafka.FirstOffset); err != nil {
		ice.logger.WithError(err).Warn("Failed to seek to beginning of Kafka topic")
		// Continue anyway - might be reading from current offset
	}

	var events []*ConversationEvent
	readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Read messages until timeout or EOF
	for {
		msg, err := reader.ReadMessage(readCtx)
		if err != nil {
			// Break on timeout or other errors
			if err == context.DeadlineExceeded || err == context.Canceled {
				ice.logger.WithError(err).Debug("Stopped reading Kafka messages")
				break
			}
			ice.logger.WithError(err).Warn("Error reading Kafka message")
			break
		}

		// Parse message value as ConversationEvent
		var event ConversationEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			ice.logger.WithError(err).WithField("offset", msg.Offset).Warn("Failed to parse Kafka message as ConversationEvent")
			continue
		}

		// Filter by conversation ID
		if event.ConversationID == conversationID {
			events = append(events, &event)
		}

		// Safety limit - break after reading many messages
		if len(events) > 10000 {
			ice.logger.Warn("Reached safety limit of 10000 events, stopping read")
			break
		}
	}

	ice.logger.WithFields(logrus.Fields{
		"conversation_id": conversationID,
		"event_count":     len(events),
		"brokers":         brokers,
		"topic":           topic,
	}).Debug("Fetched conversation events from Kafka")

	return events, nil
}

// reconstructFromEvents reconstructs conversation from events
func (ice *InfiniteContextEngine) reconstructFromEvents(events []*ConversationEvent) ([]MessageData, []EntityData) {
	// Sort events by sequence number
	sort.Slice(events, func(i, j int) bool {
		return events[i].SequenceNumber < events[j].SequenceNumber
	})

	messages := []MessageData{}
	entityMap := make(map[string]EntityData)

	for _, event := range events {
		switch event.EventType {
		case ConversationEventMessageAdded:
			if event.Message != nil {
				messages = append(messages, *event.Message)
			}

		case ConversationEventEntityExtracted:
			for _, entity := range event.Entities {
				entityMap[entity.EntityID] = entity
			}

		case ConversationEventDebateRound:
			if event.DebateRound != nil {
				// Add debate round as assistant message
				messages = append(messages, MessageData{
					MessageID: event.DebateRound.RoundID,
					Role:      "assistant",
					Content:   event.DebateRound.Response,
					Model:     event.DebateRound.Model,
					Tokens:    event.DebateRound.TokensUsed,
					CreatedAt: event.DebateRound.CreatedAt,
				})
			}

		case ConversationEventCompressed:
			// Compression events handled separately
			continue
		}
	}

	// Convert entity map to slice
	entities := make([]EntityData, 0, len(entityMap))
	for _, entity := range entityMap {
		entities = append(entities, entity)
	}

	return messages, entities
}

// calculateContext calculates context data from messages and entities
func (ice *InfiniteContextEngine) calculateContext(messages []MessageData, entities []EntityData) *ContextData {
	totalTokens := ice.countTokens(messages)

	return &ContextData{
		MessageCount:      len(messages),
		TotalTokens:       totalTokens,
		EntityCount:       len(entities),
		ContextWindow:     128000, // Default context window
		ContextUsageRatio: float64(totalTokens) / 128000.0,
	}
}

// countTokens estimates token count from messages
func (ice *InfiniteContextEngine) countTokens(messages []MessageData) int64 {
	var total int64
	for _, msg := range messages {
		if msg.Tokens > 0 {
			total += int64(msg.Tokens)
		} else {
			// Estimate: ~4 characters per token
			total += int64(len(msg.Content) / 4)
		}
	}
	return total
}

// ContextCache methods

// Get retrieves cached context
func (cc *ContextCache) Get(conversationID string) *CachedContext {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	cached, exists := cc.cache[conversationID]
	if !exists {
		return nil
	}

	// Check TTL
	if time.Since(cached.CachedAt) > cc.ttl {
		// Expired
		delete(cc.cache, conversationID)
		return nil
	}

	cached.AccessCount++
	return cached
}

// Put stores context in cache
func (cc *ContextCache) Put(conversationID string, context *CachedContext) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// Evict oldest if cache full
	if len(cc.cache) >= cc.maxSize {
		cc.evictOldest()
	}

	cc.cache[conversationID] = context
}

// evictOldest removes the least recently accessed item
func (cc *ContextCache) evictOldest() {
	var oldestID string
	var oldestTime time.Time = time.Now()

	for id, cached := range cc.cache {
		if cached.CachedAt.Before(oldestTime) {
			oldestTime = cached.CachedAt
			oldestID = id
		}
	}

	if oldestID != "" {
		delete(cc.cache, oldestID)
	}
}

// Clear clears the cache
func (cc *ContextCache) Clear() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.cache = make(map[string]*CachedContext)
}

// Size returns cache size
func (cc *ContextCache) Size() int {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return len(cc.cache)
}
