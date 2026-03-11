package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ContextCompressor compresses conversation context using LLM-based summarization
type ContextCompressor struct {
	llmClient LLMClient
	logger    *logrus.Logger
}

// LLMClient interface for LLM providers
type LLMClient interface {
	Complete(ctx context.Context, prompt string, maxTokens int) (string, int, error)
}

// CompressionStrategy defines how to compress conversations
type CompressionStrategy string

const (
	// CompressionStrategyWindowSummary summarizes message windows
	CompressionStrategyWindowSummary CompressionStrategy = "window_summary"

	// CompressionStrategyEntityGraph preserves entity relationships
	CompressionStrategyEntityGraph CompressionStrategy = "entity_graph"

	// CompressionStrategyFull full conversation summary
	CompressionStrategyFull CompressionStrategy = "full"

	// CompressionStrategyHybrid combines multiple strategies
	CompressionStrategyHybrid CompressionStrategy = "hybrid"
)

// CompressionConfig configures compression behavior
type CompressionConfig struct {
	Strategy         CompressionStrategy
	WindowSize       int     // Messages per window for window_summary
	TargetRatio      float64 // Target compression ratio (0.0-1.0)
	PreserveEntities bool    // Whether to preserve entity mentions
	PreserveTopics   bool    // Whether to preserve key topics
	LLMModel         string  // LLM model to use for summarization
}

// DefaultCompressionConfig returns default configuration
func DefaultCompressionConfig() *CompressionConfig {
	return &CompressionConfig{
		Strategy:         CompressionStrategyHybrid,
		WindowSize:       10,
		TargetRatio:      0.3, // Compress to 30% of original
		PreserveEntities: true,
		PreserveTopics:   true,
		LLMModel:         "gpt-4-turbo",
	}
}

// NewContextCompressor creates a new context compressor
func NewContextCompressor(llmClient LLMClient, logger *logrus.Logger) *ContextCompressor {
	if logger == nil {
		logger = logrus.New()
	}

	return &ContextCompressor{
		llmClient: llmClient,
		logger:    logger,
	}
}

// Compress compresses conversation to fit within token limit
func (cc *ContextCompressor) Compress(
	ctx context.Context,
	messages []MessageData,
	entities []EntityData,
	maxTokens int,
) ([]MessageData, *CompressionData, error) {
	startTime := time.Now()

	config := DefaultCompressionConfig()

	cc.logger.WithFields(logrus.Fields{
		"message_count": len(messages),
		"entity_count":  len(entities),
		"max_tokens":    maxTokens,
		"strategy":      config.Strategy,
	}).Info("Starting conversation compression")

	// Calculate original token count
	originalTokens := cc.countTokens(messages)

	// Determine compression strategy
	var compressed []MessageData
	var err error

	switch config.Strategy {
	case CompressionStrategyWindowSummary:
		compressed, err = cc.compressWindowSummary(ctx, messages, entities, config)
	case CompressionStrategyEntityGraph:
		compressed, err = cc.compressEntityGraph(ctx, messages, entities, config)
	case CompressionStrategyFull:
		compressed, err = cc.compressFull(ctx, messages, entities, config)
	case CompressionStrategyHybrid:
		compressed, err = cc.compressHybrid(ctx, messages, entities, maxTokens, config)
	default:
		return nil, nil, fmt.Errorf("unknown compression strategy: %s", config.Strategy)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("compression failed: %w", err)
	}

	// Calculate compressed token count
	compressedTokens := cc.countTokens(compressed)

	// Calculate compression ratio
	compressionRatio := float64(compressedTokens) / float64(originalTokens)

	// Extract preserved entities
	preservedEntities := cc.extractPreservedEntities(compressed, entities)

	compressionData := &CompressionData{
		CompressionID:       fmt.Sprintf("comp-%d", time.Now().UnixNano()),
		CompressionType:     string(config.Strategy),
		OriginalMessages:    len(messages),
		CompressedMessages:  len(compressed),
		OriginalTokens:      originalTokens,
		CompressedTokens:    compressedTokens,
		CompressionRatio:    compressionRatio,
		PreservedEntities:   preservedEntities,
		LLMModel:            config.LLMModel,
		CompressionDuration: time.Since(startTime).Milliseconds(),
		CompressedAt:        time.Now(),
	}

	cc.logger.WithFields(logrus.Fields{
		"original_messages":   len(messages),
		"compressed_messages": len(compressed),
		"original_tokens":     originalTokens,
		"compressed_tokens":   compressedTokens,
		"compression_ratio":   compressionRatio,
		"duration_ms":         compressionData.CompressionDuration,
	}).Info("Compression completed")

	return compressed, compressionData, nil
}

// compressWindowSummary compresses by summarizing message windows
func (cc *ContextCompressor) compressWindowSummary(
	ctx context.Context,
	messages []MessageData,
	entities []EntityData,
	config *CompressionConfig,
) ([]MessageData, error) {
	compressed := []MessageData{}

	// Keep recent messages intact
	recentCount := min(50, len(messages)/4)
	recentStart := len(messages) - recentCount

	// Compress older messages in windows
	for i := 0; i < recentStart; i += config.WindowSize {
		end := min(i+config.WindowSize, recentStart)
		window := messages[i:end]

		// Summarize window
		summary, tokens, err := cc.summarizeWindow(ctx, window, entities)
		if err != nil {
			cc.logger.WithError(err).Warn("Failed to summarize window, keeping original")
			compressed = append(compressed, window...)
			continue
		}

		// Add summary as system message
		compressed = append(compressed, MessageData{
			MessageID: fmt.Sprintf("summary-%d-%d", i, end),
			Role:      "system",
			Content:   summary,
			Model:     config.LLMModel,
			Tokens:    tokens,
			CreatedAt: window[0].CreatedAt,
		})
	}

	// Add recent messages unchanged
	compressed = append(compressed, messages[recentStart:]...)

	return compressed, nil
}

// compressEntityGraph compresses while preserving entity relationships
func (cc *ContextCompressor) compressEntityGraph(
	ctx context.Context,
	messages []MessageData,
	entities []EntityData,
	config *CompressionConfig,
) ([]MessageData, error) {
	// Keep entity-rich messages, summarize others
	compressed := []MessageData{}
	buffer := []MessageData{}

	for _, msg := range messages {
		if cc.hasEntities(msg, entities) || len(buffer) >= config.WindowSize {
			// Summarize buffer first if non-empty
			if len(buffer) > 0 {
				summary, tokens, err := cc.summarizeWindow(ctx, buffer, entities)
				if err == nil {
					compressed = append(compressed, MessageData{
						MessageID: fmt.Sprintf("summary-%d", len(compressed)),
						Role:      "system",
						Content:   summary,
						Tokens:    tokens,
						CreatedAt: buffer[0].CreatedAt,
					})
				} else {
					compressed = append(compressed, buffer...)
				}
				buffer = []MessageData{}
			}

			// Keep entity message
			compressed = append(compressed, msg)
		} else {
			buffer = append(buffer, msg)
		}
	}

	// Summarize remaining buffer
	if len(buffer) > 0 {
		summary, tokens, err := cc.summarizeWindow(ctx, buffer, entities)
		if err == nil {
			compressed = append(compressed, MessageData{
				MessageID: fmt.Sprintf("summary-%d", len(compressed)),
				Role:      "system",
				Content:   summary,
				Tokens:    tokens,
				CreatedAt: buffer[0].CreatedAt,
			})
		} else {
			compressed = append(compressed, buffer...)
		}
	}

	return compressed, nil
}

// compressFull creates full conversation summary
func (cc *ContextCompressor) compressFull(
	ctx context.Context,
	messages []MessageData,
	entities []EntityData,
	config *CompressionConfig,
) ([]MessageData, error) {
	// Create comprehensive summary
	summary, tokens, err := cc.summarizeConversation(ctx, messages, entities)
	if err != nil {
		return nil, err
	}

	// Keep summary + recent messages
	recentCount := min(20, len(messages)/5)
	compressed := []MessageData{
		{
			MessageID: "full-summary",
			Role:      "system",
			Content:   summary,
			Model:     config.LLMModel,
			Tokens:    tokens,
			CreatedAt: messages[0].CreatedAt,
		},
	}

	compressed = append(compressed, messages[len(messages)-recentCount:]...)

	return compressed, nil
}

// compressHybrid uses hybrid compression strategy
func (cc *ContextCompressor) compressHybrid(
	ctx context.Context,
	messages []MessageData,
	entities []EntityData,
	maxTokens int,
	config *CompressionConfig,
) ([]MessageData, error) {
	// Start with entity graph compression
	compressed, err := cc.compressEntityGraph(ctx, messages, entities, config)
	if err != nil {
		return nil, err
	}

	// Check if still too large
	tokens := cc.countTokens(compressed)
	if tokens > int64(maxTokens) {
		// Apply additional window summary compression
		compressed, err = cc.compressWindowSummary(ctx, compressed, entities, config)
		if err != nil {
			return nil, err
		}
	}

	// If still too large, use full summary
	tokens = cc.countTokens(compressed)
	if tokens > int64(maxTokens) {
		compressed, err = cc.compressFull(ctx, messages, entities, config)
	}

	return compressed, err
}

// summarizeWindow summarizes a window of messages using LLM
func (cc *ContextCompressor) summarizeWindow(
	ctx context.Context,
	window []MessageData,
	entities []EntityData,
) (string, int, error) {
	if len(window) == 0 {
		return "", 0, nil
	}

	// Build conversation context
	var conversation strings.Builder
	for _, msg := range window {
		conversation.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}

	// Build entity context
	entityNames := cc.extractEntityNames(window, entities)
	entityContext := ""
	if len(entityNames) > 0 {
		entityContext = fmt.Sprintf("Key entities mentioned: %s\n", strings.Join(entityNames, ", "))
	}

	// Create summarization prompt
	prompt := fmt.Sprintf(`Summarize the following conversation window while preserving:
1. Key decisions and conclusions
2. Important entity mentions and relationships
3. Action items and next steps
4. Critical context for future reference

%s
Conversation:
%s

Summary (2-3 sentences):`, entityContext, conversation.String())

	// Call LLM for summarization
	if cc.llmClient == nil {
		// Fallback: simple concatenation
		return fmt.Sprintf("[Summary of %d messages]", len(window)), 10, nil
	}

	summary, tokens, err := cc.llmClient.Complete(ctx, prompt, 200)
	if err != nil {
		return "", 0, fmt.Errorf("LLM summarization failed: %w", err)
	}

	return summary, tokens, nil
}

// summarizeConversation creates a comprehensive conversation summary
func (cc *ContextCompressor) summarizeConversation(
	ctx context.Context,
	messages []MessageData,
	entities []EntityData,
) (string, int, error) {
	// Similar to summarizeWindow but more comprehensive
	// Build full conversation
	var conversation strings.Builder
	for _, msg := range messages {
		conversation.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}

	entityNames := cc.extractEntityNames(messages, entities)
	entityContext := ""
	if len(entityNames) > 0 {
		entityContext = fmt.Sprintf("Entities involved: %s\n", strings.Join(entityNames, ", "))
	}

	prompt := fmt.Sprintf(`Create a comprehensive summary of this conversation:

%s
Total messages: %d

Conversation:
%s

Summary (5-10 sentences covering main points, decisions, and context):`,
		entityContext, len(messages), conversation.String())

	if cc.llmClient == nil {
		return fmt.Sprintf("[Summary of %d messages with %d entities]", len(messages), len(entities)), 20, nil
	}

	summary, tokens, err := cc.llmClient.Complete(ctx, prompt, 500)
	if err != nil {
		return "", 0, fmt.Errorf("LLM summarization failed: %w", err)
	}

	return summary, tokens, nil
}

// Helper methods

func (cc *ContextCompressor) countTokens(messages []MessageData) int64 {
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

func (cc *ContextCompressor) hasEntities(msg MessageData, entities []EntityData) bool {
	content := strings.ToLower(msg.Content)
	for _, entity := range entities {
		if strings.Contains(content, strings.ToLower(entity.Name)) ||
			strings.Contains(content, strings.ToLower(entity.Value)) {
			return true
		}
	}
	return false
}

func (cc *ContextCompressor) extractEntityNames(messages []MessageData, entities []EntityData) []string {
	names := make(map[string]bool)
	for _, msg := range messages {
		for _, entity := range entities {
			if cc.hasEntities(msg, []EntityData{entity}) {
				names[entity.Name] = true
			}
		}
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	return result
}

func (cc *ContextCompressor) extractPreservedEntities(messages []MessageData, entities []EntityData) []string {
	return cc.extractEntityNames(messages, entities)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
