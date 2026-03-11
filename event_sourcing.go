package conversation

import (
	"encoding/json"
	"time"
)

// ConversationEventType represents the type of conversation event
type ConversationEventType string

const (
	// Message events
	ConversationEventMessageAdded   ConversationEventType = "conversation.message.added"
	ConversationEventMessageUpdated ConversationEventType = "conversation.message.updated"
	ConversationEventMessageDeleted ConversationEventType = "conversation.message.deleted"

	// Conversation lifecycle events
	ConversationEventCreated   ConversationEventType = "conversation.created"
	ConversationEventCompleted ConversationEventType = "conversation.completed"
	ConversationEventArchived  ConversationEventType = "conversation.archived"

	// Entity and context events
	ConversationEventEntityExtracted ConversationEventType = "conversation.entity.extracted"
	ConversationEventContextUpdated  ConversationEventType = "conversation.context.updated"
	ConversationEventCompressed      ConversationEventType = "conversation.compressed"

	// Debate events
	ConversationEventDebateRound  ConversationEventType = "conversation.debate.round"
	ConversationEventDebateWinner ConversationEventType = "conversation.debate.winner"
)

// ConversationEvent represents a conversation change event for Kafka
type ConversationEvent struct {
	// Event metadata
	EventID        string                `json:"event_id"`
	EventType      ConversationEventType `json:"event_type"`
	Timestamp      time.Time             `json:"timestamp"`
	NodeID         string                `json:"node_id"` // Source node
	SequenceNumber int64                 `json:"sequence_number"`

	// Conversation identification
	ConversationID string `json:"conversation_id"`
	SessionID      string `json:"session_id,omitempty"`
	UserID         string `json:"user_id"`

	// Message data
	Message *MessageData `json:"message,omitempty"`

	// Entity data
	Entities []EntityData `json:"entities,omitempty"`

	// Context data
	Context *ContextData `json:"context,omitempty"`

	// Compression data
	Compression *CompressionData `json:"compression,omitempty"`

	// Debate data
	DebateRound *DebateRoundData `json:"debate_round,omitempty"`

	// Metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// MessageData represents message information in the event
type MessageData struct {
	MessageID string    `json:"message_id"`
	Role      string    `json:"role"` // user, assistant, system
	Content   string    `json:"content"`
	Model     string    `json:"model,omitempty"`
	Tokens    int       `json:"tokens,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// EntityData represents extracted entity information
type EntityData struct {
	EntityID   string                 `json:"entity_id"`
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	Value      string                 `json:"value,omitempty"`
	Confidence float64                `json:"confidence"`
	Context    string                 `json:"context,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// ContextData represents conversation context snapshot
type ContextData struct {
	MessageCount      int                    `json:"message_count"`
	TotalTokens       int64                  `json:"total_tokens"`
	EntityCount       int                    `json:"entity_count"`
	CompressedCount   int                    `json:"compressed_count"`
	CompressionRatio  float64                `json:"compression_ratio,omitempty"`
	KeyTopics         []string               `json:"key_topics,omitempty"`
	ActiveEntities    []string               `json:"active_entities,omitempty"`
	ContextWindow     int                    `json:"context_window"`
	ContextUsageRatio float64                `json:"context_usage_ratio"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

// CompressionData represents context compression event data
type CompressionData struct {
	CompressionID       string    `json:"compression_id"`
	CompressionType     string    `json:"compression_type"` // window_summary, entity_graph, full
	OriginalMessages    int       `json:"original_messages"`
	CompressedMessages  int       `json:"compressed_messages"`
	OriginalTokens      int64     `json:"original_tokens"`
	CompressedTokens    int64     `json:"compressed_tokens"`
	CompressionRatio    float64   `json:"compression_ratio"`
	SummaryContent      string    `json:"summary_content,omitempty"`
	PreservedEntities   []string  `json:"preserved_entities,omitempty"`
	LLMModel            string    `json:"llm_model,omitempty"`
	CompressionDuration int64     `json:"compression_duration_ms"`
	CompressedAt        time.Time `json:"compressed_at"`
}

// DebateRoundData represents debate round information
type DebateRoundData struct {
	RoundID        string    `json:"round_id"`
	RoundNumber    int       `json:"round_number"`
	Provider       string    `json:"provider"`
	Model          string    `json:"model"`
	Role           string    `json:"role"` // proposer, critic, reviewer, synthesizer
	Response       string    `json:"response"`
	TokensUsed     int       `json:"tokens_used"`
	ResponseTimeMs int64     `json:"response_time_ms"`
	Confidence     float64   `json:"confidence"`
	CreatedAt      time.Time `json:"created_at"`
}

// ConversationSnapshot represents a complete conversation state snapshot
type ConversationSnapshot struct {
	SnapshotID     string                 `json:"snapshot_id"`
	ConversationID string                 `json:"conversation_id"`
	UserID         string                 `json:"user_id"`
	SessionID      string                 `json:"session_id,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
	NodeID         string                 `json:"node_id"`
	SequenceNumber int64                  `json:"sequence_number"`
	Messages       []MessageData          `json:"messages"`
	Entities       []EntityData           `json:"entities"`
	Context        *ContextData           `json:"context"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// EventStream represents a stream of conversation events
type EventStream struct {
	StreamID       string               `json:"stream_id"`
	ConversationID string               `json:"conversation_id"`
	UserID         string               `json:"user_id"`
	SessionID      string               `json:"session_id,omitempty"`
	StartTime      time.Time            `json:"start_time"`
	EndTime        time.Time            `json:"end_time,omitempty"`
	Events         []*ConversationEvent `json:"events"`
	EventCount     int                  `json:"event_count"`
	Checkpoints    []time.Time          `json:"checkpoints,omitempty"`
}

// NewConversationEvent creates a new conversation event
func NewConversationEvent(eventType ConversationEventType, nodeID, conversationID, userID string) *ConversationEvent {
	return &ConversationEvent{
		EventID:        generateEventID(),
		EventType:      eventType,
		Timestamp:      time.Now().UTC(),
		NodeID:         nodeID,
		ConversationID: conversationID,
		UserID:         userID,
		SequenceNumber: time.Now().UnixNano(),
	}
}

// ToJSON converts the event to JSON
func (e *ConversationEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// FromJSON parses an event from JSON
func ConversationEventFromJSON(data []byte) (*ConversationEvent, error) {
	var event ConversationEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// Clone creates a deep copy of the event
func (e *ConversationEvent) Clone() *ConversationEvent {
	clone := &ConversationEvent{
		EventID:        e.EventID,
		EventType:      e.EventType,
		Timestamp:      e.Timestamp,
		NodeID:         e.NodeID,
		SequenceNumber: e.SequenceNumber,
		ConversationID: e.ConversationID,
		SessionID:      e.SessionID,
		UserID:         e.UserID,
	}

	// Deep copy message
	if e.Message != nil {
		clone.Message = &MessageData{
			MessageID: e.Message.MessageID,
			Role:      e.Message.Role,
			Content:   e.Message.Content,
			Model:     e.Message.Model,
			Tokens:    e.Message.Tokens,
			CreatedAt: e.Message.CreatedAt,
		}
	}

	// Deep copy entities
	if len(e.Entities) > 0 {
		clone.Entities = make([]EntityData, len(e.Entities))
		copy(clone.Entities, e.Entities)
	}

	// Deep copy context
	if e.Context != nil {
		clone.Context = &ContextData{
			MessageCount:      e.Context.MessageCount,
			TotalTokens:       e.Context.TotalTokens,
			EntityCount:       e.Context.EntityCount,
			CompressedCount:   e.Context.CompressedCount,
			CompressionRatio:  e.Context.CompressionRatio,
			ContextWindow:     e.Context.ContextWindow,
			ContextUsageRatio: e.Context.ContextUsageRatio,
		}
		if len(e.Context.KeyTopics) > 0 {
			clone.Context.KeyTopics = make([]string, len(e.Context.KeyTopics))
			copy(clone.Context.KeyTopics, e.Context.KeyTopics)
		}
		if len(e.Context.ActiveEntities) > 0 {
			clone.Context.ActiveEntities = make([]string, len(e.Context.ActiveEntities))
			copy(clone.Context.ActiveEntities, e.Context.ActiveEntities)
		}
	}

	// Deep copy compression
	if e.Compression != nil {
		clone.Compression = &CompressionData{
			CompressionID:       e.Compression.CompressionID,
			CompressionType:     e.Compression.CompressionType,
			OriginalMessages:    e.Compression.OriginalMessages,
			CompressedMessages:  e.Compression.CompressedMessages,
			OriginalTokens:      e.Compression.OriginalTokens,
			CompressedTokens:    e.Compression.CompressedTokens,
			CompressionRatio:    e.Compression.CompressionRatio,
			SummaryContent:      e.Compression.SummaryContent,
			LLMModel:            e.Compression.LLMModel,
			CompressionDuration: e.Compression.CompressionDuration,
			CompressedAt:        e.Compression.CompressedAt,
		}
		if len(e.Compression.PreservedEntities) > 0 {
			clone.Compression.PreservedEntities = make([]string, len(e.Compression.PreservedEntities))
			copy(clone.Compression.PreservedEntities, e.Compression.PreservedEntities)
		}
	}

	// Deep copy debate round
	if e.DebateRound != nil {
		clone.DebateRound = &DebateRoundData{
			RoundID:        e.DebateRound.RoundID,
			RoundNumber:    e.DebateRound.RoundNumber,
			Provider:       e.DebateRound.Provider,
			Model:          e.DebateRound.Model,
			Role:           e.DebateRound.Role,
			Response:       e.DebateRound.Response,
			TokensUsed:     e.DebateRound.TokensUsed,
			ResponseTimeMs: e.DebateRound.ResponseTimeMs,
			Confidence:     e.DebateRound.Confidence,
			CreatedAt:      e.DebateRound.CreatedAt,
		}
	}

	// Deep copy metadata
	if e.Metadata != nil {
		clone.Metadata = make(map[string]interface{})
		for k, v := range e.Metadata {
			clone.Metadata[k] = v
		}
	}

	return clone
}

// generateEventID generates a unique event ID
func generateEventID() string {
	// Format: cevt-<timestamp>-<random>
	return "cevt-" + time.Now().UTC().Format("20060102150405.000000") + "-" + randomString(8)
}

// randomString generates a random string of specified length
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
