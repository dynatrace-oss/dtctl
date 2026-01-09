package copilot

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles Davis CoPilot resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new copilot handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// Skill represents an available CoPilot skill (just a string type)
type Skill struct {
	Name string `table:"NAME"`
}

// SkillsResponse represents the list of available skills
type SkillsResponse struct {
	Skills []string `json:"skills"`
}

// SkillList is the processed list for display
type SkillList struct {
	Skills []Skill
}

// ConversationRequest represents a request to the CoPilot conversation endpoint
type ConversationRequest struct {
	Text    string                `json:"text"`
	State   *ConversationState    `json:"state,omitempty"`
	Context []ConversationContext `json:"context,omitempty"`
}

// ConversationState represents the conversation state for multi-turn conversations
type ConversationState struct {
	Messages []ConversationMessage `json:"messages,omitempty"`
}

// ConversationMessage represents a message in the conversation history
type ConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ConversationContext represents a context item for the conversation
type ConversationContext struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// ConversationResponse represents a response from the CoPilot conversation endpoint
type ConversationResponse struct {
	Text  string             `json:"text" table:"RESPONSE"`
	State *ConversationState `json:"state,omitempty" table:"-"`
}

// StreamChunk represents a chunk in a streaming response (ndjson event format)
type StreamChunk struct {
	Event string           `json:"event"`
	Data  *StreamChunkData `json:"data,omitempty"`
}

// StreamChunkData represents the data field in a streaming chunk
type StreamChunkData struct {
	Tokens       []string           `json:"tokens,omitempty"`
	Text         string             `json:"text,omitempty"`
	State        *ConversationState `json:"state,omitempty"`
	MessageToken string             `json:"messageToken,omitempty"`
	Type         string             `json:"type,omitempty"`
	Message      string             `json:"message,omitempty"`
}

// ListSkills retrieves all available CoPilot skills
func (h *Handler) ListSkills() (*SkillList, error) {
	var result SkillsResponse

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get("/platform/davis/copilot/v1/skills")

	if err != nil {
		return nil, fmt.Errorf("failed to list skills: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list skills: status %d: %s", resp.StatusCode(), resp.String())
	}

	// Convert string array to Skill structs for display
	skills := make([]Skill, len(result.Skills))
	for i, s := range result.Skills {
		skills[i] = Skill{Name: s}
	}

	return &SkillList{Skills: skills}, nil
}

// Chat sends a message to CoPilot and returns the response
func (h *Handler) Chat(text string, state *ConversationState, ctx []ConversationContext) (*ConversationResponse, error) {
	req := ConversationRequest{
		Text:    text,
		State:   state,
		Context: ctx,
	}

	var result ConversationResponse

	resp, err := h.client.HTTP().R().
		SetHeader("Accept", "application/json").
		SetBody(req).
		SetResult(&result).
		Post("/platform/davis/copilot/v1/skills/conversations:message")

	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to send message: status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// ChatStream sends a message to CoPilot and streams the response
func (h *Handler) ChatStream(text string, state *ConversationState, ctx []ConversationContext, callback func(chunk StreamChunk) error) (*ConversationResponse, error) {
	req := ConversationRequest{
		Text:    text,
		State:   state,
		Context: ctx,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := h.client.HTTP().R().
		SetHeader("Accept", "application/x-ndjson").
		SetHeader("Content-Type", "application/json").
		SetBody(reqBody).
		SetDoNotParseResponse(true).
		Post("/platform/davis/copilot/v1/skills/conversations:message")

	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	if resp.IsError() {
		body, _ := io.ReadAll(resp.RawBody())
		resp.RawBody().Close()
		return nil, fmt.Errorf("failed to send message: status %d: %s", resp.StatusCode(), string(body))
	}

	defer resp.RawBody().Close()

	var fullText strings.Builder
	var finalState *ConversationState

	scanner := bufio.NewScanner(resp.RawBody())
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}

		// Handle tokens event - accumulate text
		if chunk.Data != nil && len(chunk.Data.Tokens) > 0 {
			for _, token := range chunk.Data.Tokens {
				fullText.WriteString(token)
			}
		}

		// Handle state from end event
		if chunk.Data != nil && chunk.Data.State != nil {
			finalState = chunk.Data.State
		}

		if callback != nil {
			if err := callback(chunk); err != nil {
				return nil, err
			}
		}

		// End event signals completion
		if chunk.Event == "end" {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading stream: %w", err)
	}

	return &ConversationResponse{
		Text:  fullText.String(),
		State: finalState,
	}, nil
}

// ChatOptions holds options for chat operations
type ChatOptions struct {
	Stream            bool
	DocumentRetrieval string
	Supplementary     string
	Instruction       string
	State             *ConversationState
}

// ChatWithOptions sends a message with options
func (h *Handler) ChatWithOptions(text string, opts ChatOptions, streamCallback func(chunk StreamChunk) error) (*ConversationResponse, error) {
	var ctx []ConversationContext
	if opts.DocumentRetrieval != "" {
		ctx = append(ctx, ConversationContext{Type: "document-retrieval", Value: opts.DocumentRetrieval})
	}
	if opts.Supplementary != "" {
		ctx = append(ctx, ConversationContext{Type: "supplementary", Value: opts.Supplementary})
	}
	if opts.Instruction != "" {
		ctx = append(ctx, ConversationContext{Type: "instruction", Value: opts.Instruction})
	}

	if opts.Stream {
		return h.ChatStream(text, opts.State, ctx, streamCallback)
	}

	return h.Chat(text, opts.State, ctx)
}

// Nl2DqlRequest represents a request to convert natural language to DQL
type Nl2DqlRequest struct {
	Text string `json:"text"`
}

// Nl2DqlResponse represents the response from the NL to DQL skill
type Nl2DqlResponse struct {
	DQL          string `json:"dql" table:"DQL"`
	MessageToken string `json:"messageToken" table:"-"`
	Status       string `json:"status" table:"STATUS"`
}

// Nl2Dql converts natural language to a DQL query
func (h *Handler) Nl2Dql(text string) (*Nl2DqlResponse, error) {
	req := Nl2DqlRequest{Text: text}
	var result Nl2DqlResponse

	resp, err := h.client.HTTP().R().
		SetBody(req).
		SetResult(&result).
		Post("/platform/davis/copilot/v1/skills/nl2dql:generate")

	if err != nil {
		return nil, fmt.Errorf("failed to generate DQL: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to generate DQL: status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// Dql2NlRequest represents a request to explain a DQL query
type Dql2NlRequest struct {
	DQL string `json:"dql"`
}

// Dql2NlResponse represents the response from the DQL to NL skill
type Dql2NlResponse struct {
	Summary      string `json:"summary" table:"SUMMARY"`
	Explanation  string `json:"explanation" table:"EXPLANATION"`
	MessageToken string `json:"messageToken" table:"-"`
	Status       string `json:"status" table:"STATUS"`
}

// Dql2Nl explains a DQL query in natural language
func (h *Handler) Dql2Nl(dql string) (*Dql2NlResponse, error) {
	req := Dql2NlRequest{DQL: dql}
	var result Dql2NlResponse

	resp, err := h.client.HTTP().R().
		SetBody(req).
		SetResult(&result).
		Post("/platform/davis/copilot/v1/skills/dql2nl:explain")

	if err != nil {
		return nil, fmt.Errorf("failed to explain DQL: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to explain DQL: status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// DocumentSearchRequest represents a request to search for documents
type DocumentSearchRequest struct {
	Texts       []string `json:"texts"`
	Collections []string `json:"collections"`
	Exclude     []string `json:"exclude,omitempty"`
}

// DocumentMetadata represents metadata about a document
type DocumentMetadata struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
}

// ScoredDocument represents a document with its relevance score
type ScoredDocument struct {
	DocumentID       string           `json:"documentId" table:"ID"`
	RelevanceScore   float64          `json:"relevanceScore" table:"SCORE"`
	DocumentMetadata DocumentMetadata `json:"documentMetadata" table:"-"`
	Name             string           `table:"NAME"`
	Type             string           `table:"TYPE"`
}

// DocumentSearchResponse represents the response from document search
type DocumentSearchResponse struct {
	MessageToken string           `json:"messageToken"`
	Results      []ScoredDocument `json:"results"`
	Status       string           `json:"status"`
}

// DocumentSearchResult is a processed result for display
type DocumentSearchResult struct {
	Documents []ScoredDocument
	Status    string
}

// DocumentSearch searches for relevant documents
func (h *Handler) DocumentSearch(texts []string, collections []string, exclude []string) (*DocumentSearchResult, error) {
	req := DocumentSearchRequest{
		Texts:       texts,
		Collections: collections,
		Exclude:     exclude,
	}
	var result DocumentSearchResponse

	resp, err := h.client.HTTP().R().
		SetBody(req).
		SetResult(&result).
		Post("/platform/davis/copilot/v1/skills/document-search:execute")

	if err != nil {
		return nil, fmt.Errorf("failed to search documents: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to search documents: status %d: %s", resp.StatusCode(), resp.String())
	}

	// Populate display fields from metadata
	for i := range result.Results {
		result.Results[i].Name = result.Results[i].DocumentMetadata.Name
		result.Results[i].Type = result.Results[i].DocumentMetadata.Type
	}

	return &DocumentSearchResult{
		Documents: result.Results,
		Status:    result.Status,
	}, nil
}
