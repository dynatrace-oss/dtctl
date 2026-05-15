package copilot

import (
	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkcop "github.com/dynatrace-oss/dtctl/sdk/api/copilot"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	Skill                  = sdkcop.Skill
	SkillsResponse         = sdkcop.SkillsResponse
	SkillList              = sdkcop.SkillList
	ConversationRequest    = sdkcop.ConversationRequest
	ConversationState      = sdkcop.ConversationState
	ConversationMessage    = sdkcop.ConversationMessage
	ConversationContext    = sdkcop.ConversationContext
	ConversationResponse   = sdkcop.ConversationResponse
	StreamChunk            = sdkcop.StreamChunk
	StreamChunkData        = sdkcop.StreamChunkData
	ChatOptions            = sdkcop.ChatOptions
	Nl2DqlRequest          = sdkcop.Nl2DqlRequest
	Nl2DqlResponse         = sdkcop.Nl2DqlResponse
	Dql2NlRequest          = sdkcop.Dql2NlRequest
	Dql2NlResponse         = sdkcop.Dql2NlResponse
	DocumentSearchRequest  = sdkcop.DocumentSearchRequest
	DocumentMetadata       = sdkcop.DocumentMetadata
	ScoredDocument         = sdkcop.ScoredDocument
	DocumentSearchResponse = sdkcop.DocumentSearchResponse
	DocumentSearchResult   = sdkcop.DocumentSearchResult
)

// Handler handles Davis CoPilot resources.
type Handler struct {
	sdk *sdkcop.Handler
}

// NewHandler creates a new copilot handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdkcop.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// ListSkills retrieves all available CoPilot skills
func (h *Handler) ListSkills() (*SkillList, error) {
	return h.sdk.ListSkills()
}

// Chat sends a message to CoPilot and returns the response
func (h *Handler) Chat(text string, state *ConversationState, ctx []ConversationContext) (*ConversationResponse, error) {
	return h.sdk.Chat(text, state, ctx)
}

// ChatStream sends a message to CoPilot and streams the response
func (h *Handler) ChatStream(text string, state *ConversationState, ctx []ConversationContext, callback func(chunk StreamChunk) error) (*ConversationResponse, error) {
	return h.sdk.ChatStream(text, state, ctx, callback)
}

// ChatWithOptions sends a message with options
func (h *Handler) ChatWithOptions(text string, opts ChatOptions, streamCallback func(chunk StreamChunk) error) (*ConversationResponse, error) {
	return h.sdk.ChatWithOptions(text, opts, streamCallback)
}

// Nl2Dql converts natural language to a DQL query
func (h *Handler) Nl2Dql(text string) (*Nl2DqlResponse, error) {
	return h.sdk.Nl2Dql(text)
}

// Dql2Nl explains a DQL query in natural language
func (h *Handler) Dql2Nl(dql string) (*Dql2NlResponse, error) {
	return h.sdk.Dql2Nl(dql)
}

// DocumentSearch searches for relevant documents
func (h *Handler) DocumentSearch(texts []string, collections []string, exclude []string) (*DocumentSearchResult, error) {
	return h.sdk.DocumentSearch(texts, collections, exclude)
}
