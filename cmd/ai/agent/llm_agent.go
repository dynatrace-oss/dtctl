package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	mcp_golang "github.com/metoro-io/mcp-golang"
	mcp_http_transport "github.com/metoro-io/mcp-golang/transport/http"

	aiconfig "github.com/dynatrace-oss/dtctl/pkg/ai/config"
)

// LLMAgent represents an LLM that can call MCP server tools
type LLMAgent struct {
	serverURL       string
	modelName       string
	provider        string
	client          *mcp_golang.Client
	httpClient      *http.Client
	apiKey          string
	providerBaseUrl string
	mcpInstructions *string
}

// NewLLMAgent creates a new LLM agent connected to an MCP server
func NewLLMAgent(serverURL string, modelName string, provider string) *LLMAgent {
	providerName := strings.ToLower(strings.TrimSpace(provider))

	baseURL := ""
	apiKey := ""
	defaultModel := ""

	switch providerName {
	case "openrouter":
		baseURL = aiconfig.GetOpenRouterBaseURL()
		apiKey = aiconfig.GetOpenRouterAPIKey()
		defaultModel = "meta-llama/llama-3.3-70b-instruct"
	case "google", "gemini":
		baseURL = aiconfig.GetGeminiBaseURL()
		apiKey = aiconfig.GetGeminiAPIKey()
		defaultModel = "gemini-2.5-flash"
		providerName = "google"
	case "deepseek":
		baseURL = aiconfig.GetDeepSeekBaseURL()
		apiKey = aiconfig.GetDeepSeekAPIKey()
		defaultModel = "deepseek-chat"
	case "anthropic", "claude":
		baseURL = aiconfig.GetAnthropicBaseURL()
		apiKey = aiconfig.GetAnthropicAPIKey()
		defaultModel = "claude-haiku-4-5"
		providerName = "anthropic"
	case "mistral":
		baseURL = aiconfig.GetMistralBaseURL()
		apiKey = aiconfig.GetMistralAPIKey()
		defaultModel = "mistral-small-latest"
	default:
		baseURL = aiconfig.GetOpenAIBaseURL()
		apiKey = aiconfig.GetOpenAIAPIKey()
		defaultModel = "gpt-4o-mini"
		providerName = "openai"
	}

	if strings.TrimSpace(modelName) == "" {
		modelName = defaultModel
	}

	return &LLMAgent{
		serverURL:       serverURL,
		modelName:       modelName,
		provider:        providerName,
		httpClient:      &http.Client{},
		apiKey:          apiKey,
		providerBaseUrl: strings.TrimRight(baseURL, "/"),
	}
}

// ClaudeMessage represents a message in Claude API format
type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ClaudeRequest represents a request to Claude API
type ClaudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []ClaudeMessage `json:"messages"`
	Tools     []ClaudeTool    `json:"tools,omitempty"`
}

// ClaudeTool represents a tool definition for Claude
type ClaudeTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

// ClaudeResponse represents Claude's response
type ClaudeResponse struct {
	Content []ContentBlock `json:"content"`
	Usage   *ClaudeUsage   `json:"usage,omitempty"`
	Error   *ErrorInfo     `json:"error,omitempty"`
}

// ClaudeUsage represents token usage in Claude's response
type ClaudeUsage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// ContentBlock represents a block in Claude's response
type ContentBlock struct {
	Type  string      `json:"type"`
	Text  string      `json:"text,omitempty"`
	ID    string      `json:"id,omitempty"`
	Name  string      `json:"name,omitempty"`
	Input interface{} `json:"input,omitempty"`
}

// ErrorInfo represents error information
type ErrorInfo struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type OpenAITool struct {
	Type     string             `json:"type"`
	Function OpenAIFunctionTool `json:"function"`
}

type OpenAIFunctionTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters"`
}

type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIFunctionCall `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIChatCompletionRequest struct {
	Model      string          `json:"model"`
	Messages   []OpenAIMessage `json:"messages"`
	MaxTokens  int             `json:"max_tokens,omitempty"`
	Tools      []OpenAITool    `json:"tools,omitempty"`
	ToolChoice string          `json:"tool_choice,omitempty"`
}

type OpenAIChatCompletionResponse struct {
	Choices []struct {
		Message OpenAIMessage `json:"message"`
	} `json:"choices"`
	Usage *OpenAIUsage `json:"usage,omitempty"`
	Error *ErrorInfo   `json:"error,omitempty"`
}

// OpenAIUsage represents token usage in OpenAI's response
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *GeminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

type GeminiFunctionCall struct {
	ID   string                 `json:"id,omitempty"`
	Name string                 `json:"name,omitempty"`
	Args map[string]interface{} `json:"args,omitempty"`
}

type GeminiFunctionResponse struct {
	ID       string                 `json:"id,omitempty"`
	Name     string                 `json:"name,omitempty"`
	Response map[string]interface{} `json:"response,omitempty"`
}

type GeminiFunctionDeclaration struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters"`
}

type GeminiTool struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type GeminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type GeminiToolConfig struct {
	FunctionCallingConfig *GeminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type GeminiGenerationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

type GeminiGenerateContentRequest struct {
	Contents         []GeminiContent         `json:"contents"`
	Tools            []GeminiTool            `json:"tools,omitempty"`
	ToolConfig       *GeminiToolConfig       `json:"toolConfig,omitempty"`
	GenerationConfig *GeminiGenerationConfig `json:"generationConfig,omitempty"`
}

type GeminiGenerateContentResponse struct {
	Candidates []struct {
		Content      GeminiContent `json:"content"`
		FinishReason string        `json:"finishReason,omitempty"`
	} `json:"candidates"`
	UsageMetadata *GeminiUsage `json:"usageMetadata,omitempty"`
}

// GeminiUsage represents token usage in Gemini's response
type GeminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount int `json:"candidatesTokenCount,omitempty"`
	TotalTokenCount      int `json:"totalTokenCount,omitempty"`
}

// AgentConversationMessage is an input message for multi-message conversations.
type AgentConversationMessage struct {
	Role    string `json:"role"`
	Message string `json:"message"`
}

// AgentSettings controls runtime behavior for the provider request.
type AgentSettings struct {
	MaxTokens int `json:"max_tokens,omitempty"`
}

// ConversationResult holds the response text and token usage
type ConversationResult struct {
	Response string
	Usage    *TokenUsage
}

// TokenUsage represents token usage across all providers
type TokenUsage struct {
	Total      int `json:"total"`
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
}

const defaultAgentMaxTokens = 1024

// ProcessPrompt processes a prompt using the configured LLM with MCP server tools
func (agent *LLMAgent) ProcessPrompt(prompt string) (string, error) {
	messages := []AgentConversationMessage{{
		Role:    "user",
		Message: prompt,
	}}

	result, err := agent.ProcessConversationWithUsage(messages, AgentSettings{})
	if err != nil {
		return "", err
	}
	return result.Response, nil
}

// ProcessConversation processes a conversation using the configured LLM with MCP server tools.
func (agent *LLMAgent) ProcessConversation(messages []AgentConversationMessage, settings AgentSettings) (string, error) {
	result, err := agent.ProcessConversationWithUsage(messages, settings)
	if err != nil {
		return "", err
	}
	return result.Response, nil
}

// ProcessConversationWithUsage processes a conversation and returns both response and token usage.
func (agent *LLMAgent) ProcessConversationWithUsage(messages []AgentConversationMessage, settings AgentSettings) (*ConversationResult, error) {
	ctx := context.Background()

	if strings.TrimSpace(agent.apiKey) == "" {
		return nil, fmt.Errorf("LLM credentials are required for provider %s", agent.provider)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("at least one message is required")
	}

	maxTokens := settings.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultAgentMaxTokens
	}

	prompt := latestUserMessage(messages)
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("at least one user message is required")
	}

	if err := agent.initializeMCPClient(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	effectiveMessages := messages
	if agent.mcpInstructions != nil && strings.TrimSpace(*agent.mcpInstructions) != "" {
		effectiveMessages = append([]AgentConversationMessage{
			{Role: "system", Message: *agent.mcpInstructions},
		}, messages...)
	}

	toolsResp, err := agent.client.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP tools: %w", err)
	}

	if toolsResp == nil || len(toolsResp.Tools) == 0 {
		return nil, fmt.Errorf("no tools available from MCP server")
	}

	dynamicRunCmdSchema := agent.buildDynamicRunCommandSchema(ctx)

	claudeTools := make([]ClaudeTool, 0, len(toolsResp.Tools))
	openAITools := make([]OpenAITool, 0, len(toolsResp.Tools))
	geminiTools := make([]GeminiTool, 0, len(toolsResp.Tools))
	for _, tool := range toolsResp.Tools {
		description := ""
		if tool.Description != nil {
			description = *tool.Description
		}

		inputSchema := map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}

		if tool.InputSchema != nil {
			if typedSchema, ok := tool.InputSchema.(map[string]interface{}); ok {
				inputSchema = typedSchema
			}
		}

		if tool.Name == "run_dtctl_command" && dynamicRunCmdSchema != nil {
			inputSchema = dynamicRunCmdSchema
		}

		geminiInputSchema := sanitizeGeminiSchema(inputSchema)

		claudeTools = append(claudeTools, ClaudeTool{
			Name:        tool.Name,
			Description: description,
			InputSchema: inputSchema,
		})

		openAITools = append(openAITools, OpenAITool{
			Type: "function",
			Function: OpenAIFunctionTool{
				Name:        tool.Name,
				Description: description,
				Parameters:  inputSchema,
			},
		})

		geminiTools = append(geminiTools, GeminiTool{
			FunctionDeclarations: []GeminiFunctionDeclaration{{
				Name:        tool.Name,
				Description: description,
				Parameters:  geminiInputSchema,
			}},
		})
	}

	openAITools = selectPreferredToolsForPrompt(
		prompt,
		openAITools,
		128,
		func(tool OpenAITool) string { return tool.Function.Name },
		func(tool OpenAITool) string { return tool.Function.Description },
	)
	claudeTools = selectPreferredToolsForPrompt(
		prompt,
		claudeTools,
		128,
		func(tool ClaudeTool) string { return tool.Name },
		func(tool ClaudeTool) string { return tool.Description },
	)
	geminiTools = selectPreferredToolsForPrompt(
		prompt,
		geminiTools,
		128,
		func(tool GeminiTool) string {
			if len(tool.FunctionDeclarations) == 0 {
				return ""
			}
			return tool.FunctionDeclarations[0].Name
		},
		func(tool GeminiTool) string {
			if len(tool.FunctionDeclarations) == 0 {
				return ""
			}
			return tool.FunctionDeclarations[0].Description
		},
	)

	var result *ConversationResult
	switch agent.provider {
	case "anthropic":
		result, err = agent.runAnthropic(ctx, effectiveMessages, maxTokens, claudeTools)
	case "google":
		result, err = agent.runGemini(ctx, effectiveMessages, maxTokens, geminiTools)
	case "mistral":
		result, err = agent.runOpenAI(ctx, effectiveMessages, maxTokens, openAITools)
	default:
		result, err = agent.runOpenAI(ctx, effectiveMessages, maxTokens, openAITools)
	}

	if err == nil && result != nil && strings.TrimSpace(result.Response) != "" {
		return result, nil
	}

	if err != nil {
		return nil, err
	}

	return &ConversationResult{Response: "No response from model", Usage: &TokenUsage{}}, nil
}

func latestUserMessage(messages []AgentConversationMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		role := strings.ToLower(strings.TrimSpace(messages[i].Role))
		if role == "user" && strings.TrimSpace(messages[i].Message) != "" {
			return messages[i].Message
		}
	}

	return ""
}

func (agent *LLMAgent) buildDynamicRunCommandSchema(ctx context.Context) map[string]interface{} {
	raw, err := agent.callTool(ctx, "list_dtctl_commands", map[string]interface{}{})
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}

	output := raw
	if idx := strings.Index(raw, "output:\n"); idx >= 0 {
		output = raw[idx+len("output:\n"):]
	}

	commands := extractTopLevelDTCTLCommands(output)
	if len(commands) == 0 {
		return nil
	}

	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Top-level dtctl command name",
				"enum":        commands,
			},
			"args": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Subcommand, resource id/name, flags and values",
			},
		},
		"required": []string{"command"},
	}
}

func extractTopLevelDTCTLCommands(helpText string) []string {
	lines := strings.Split(helpText, "\n")
	inSection := false
	unique := map[string]bool{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.TrimSpace(trimmed) == "" {
			if inSection {
				break
			}

			continue
		}

		if strings.HasPrefix(trimmed, "Available Commands:") {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}

		if strings.HasSuffix(trimmed, ":") {
			break
		}

		parts := strings.Fields(trimmed)
		if len(parts) == 0 {
			continue
		}

		cmd := strings.TrimSpace(parts[0])
		if cmd == "help" {
			continue
		}
		if strings.Contains(cmd, "[") || strings.Contains(cmd, "<") {
			continue
		}
		unique[cmd] = true
	}

	if len(unique) == 0 {
		return nil
	}

	commands := make([]string, 0, len(unique))
	for cmd := range unique {
		commands = append(commands, cmd)
	}
	sort.Strings(commands)
	return commands
}

func selectPreferredToolsForPrompt[T any](
	prompt string,
	tools []T,
	maxTools int,
	nameFn func(T) string,
	descriptionFn func(T) string,
) []T {
	if len(tools) <= maxTools || maxTools <= 0 {
		return tools
	}

	normalizedPrompt := strings.ToLower(prompt)
	replacer := strings.NewReplacer("-", " ", "_", " ", "/", " ", ",", " ", ".", " ", ":", " ", ";", " ")
	normalizedPrompt = replacer.Replace(normalizedPrompt)
	promptTokens := map[string]bool{}
	for _, part := range strings.Fields(normalizedPrompt) {
		if len(part) >= 2 {
			promptTokens[part] = true
		}
	}

	essential := map[string]bool{
		"list_dtctl_commands":    true,
		"get_dtctl_command_help": true,
		"run_dtctl_command":      true,
	}

	type scoredTool struct {
		tool  T
		score int
	}
	scored := make([]scoredTool, 0, len(tools))

	for _, tool := range tools {
		toolName := nameFn(tool)
		name := strings.ToLower(toolName)
		desc := strings.ToLower(descriptionFn(tool))
		score := 0

		if essential[strings.ToLower(toolName)] {
			score += 1000
		}

		nameForTokens := replacer.Replace(name)
		for _, token := range strings.Fields(nameForTokens) {
			if promptTokens[token] {
				score += 10
			}
		}

		for token := range promptTokens {
			if strings.Contains(name, token) {
				score += 6
			}

			if strings.TrimSpace(token) != "" && strings.Contains(desc, token) {
				score += 2
			}
		}

		scored = append(scored, scoredTool{tool: tool, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return nameFn(scored[i].tool) < nameFn(scored[j].tool)
		}
		return scored[i].score > scored[j].score
	})

	selected := make([]T, 0, maxTools)
	for i := 0; i < len(scored) && len(selected) < maxTools; i++ {
		selected = append(selected, scored[i].tool)
	}

	return selected
}

func sanitizeGeminiSchema(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		sanitized := make(map[string]interface{}, len(typed))
		for key, child := range typed {
			if key == "$schema" {
				continue
			}
			sanitized[key] = sanitizeGeminiSchema(child)
		}
		return sanitized
	case []interface{}:
		sanitized := make([]interface{}, len(typed))
		for index, child := range typed {
			sanitized[index] = sanitizeGeminiSchema(child)
		}
		return sanitized
	default:
		return value
	}
}

func (agent *LLMAgent) runAnthropic(ctx context.Context, messages []AgentConversationMessage, maxTokens int, claudeTools []ClaudeTool) (*ConversationResult, error) {
	claudeMessages := make([]ClaudeMessage, 0, len(messages))
	for _, input := range messages {
		role := strings.ToLower(strings.TrimSpace(input.Role))
		if strings.TrimSpace(input.Message) == "" {
			continue
		}

		switch role {
		case "assistant", "user":
			claudeMessages = append(claudeMessages, ClaudeMessage{Role: role, Content: input.Message})
		case "system":
			// Keep support for system role by passing it as user content for Anthropic.
			claudeMessages = append(claudeMessages, ClaudeMessage{Role: "user", Content: "System: " + input.Message})
		default:
			claudeMessages = append(claudeMessages, ClaudeMessage{Role: "user", Content: input.Message})
		}
	}

	if len(claudeMessages) == 0 {
		return nil, fmt.Errorf("at least one non-empty message is required")
	}

	claudeReq := ClaudeRequest{
		Model:     agent.modelName,
		MaxTokens: maxTokens,
		Tools:     claudeTools,
		Messages:  claudeMessages,
	}

	response, err := agent.callClaude(ctx, claudeReq)
	if err != nil {
		return nil, err
	}

	var tokenUsage *TokenUsage
	if response.Usage != nil {
		tokenUsage = &TokenUsage{
			Prompt:     response.Usage.InputTokens,
			Completion: response.Usage.OutputTokens,
			Total:      response.Usage.InputTokens + response.Usage.OutputTokens,
		}
	}

	for _, block := range response.Content {
		if block.Type != "tool_use" {
			continue
		}
		argsMap, ok := block.Input.(map[string]interface{})
		if !ok {
			inputJSON, _ := json.Marshal(block.Input)
			if unmarshalErr := json.Unmarshal(inputJSON, &argsMap); unmarshalErr != nil {
				return nil, fmt.Errorf("failed to parse tool input: %w", unmarshalErr)
			}
		}
		toolResult, err := agent.callTool(ctx, block.Name, argsMap)
		if err != nil {
			return nil, err
		}
		return &ConversationResult{Response: toolResult, Usage: tokenUsage}, nil
	}

	for _, block := range response.Content {
		if block.Type == "text" {
			return &ConversationResult{Response: block.Text, Usage: tokenUsage}, nil
		}
	}

	return &ConversationResult{Response: "", Usage: tokenUsage}, nil
}

func (agent *LLMAgent) runGemini(ctx context.Context, messages []AgentConversationMessage, maxTokens int, geminiTools []GeminiTool) (*ConversationResult, error) {
	contents := make([]GeminiContent, 0, len(messages))
	for _, input := range messages {
		if strings.TrimSpace(input.Message) == "" {
			continue
		}

		role := strings.ToLower(strings.TrimSpace(input.Role))
		switch role {
		case "model", "assistant":
			role = "model"
		default:
			role = "user"
		}

		text := input.Message
		if strings.ToLower(strings.TrimSpace(input.Role)) == "system" {
			text = "System: " + input.Message
		}

		contents = append(contents, GeminiContent{
			Role:  role,
			Parts: []GeminiPart{{Text: text}},
		})
	}

	if len(contents) == 0 {
		return nil, fmt.Errorf("at least one non-empty message is required")
	}

	req := GeminiGenerateContentRequest{
		Contents: contents,
	}
	if len(geminiTools) > 0 {
		req.Tools = geminiTools
		req.ToolConfig = &GeminiToolConfig{
			FunctionCallingConfig: &GeminiFunctionCallingConfig{Mode: "AUTO"},
		}
	}
	req.GenerationConfig = &GeminiGenerationConfig{MaxOutputTokens: maxTokens}

	for round := 0; round < 5; round++ {
		resp, err := agent.callGemini(ctx, req)
		if err != nil {
			return nil, err
		}
		if len(resp.Candidates) == 0 {
			var tokenUsage *TokenUsage
			if resp.UsageMetadata != nil {
				tokenUsage = &TokenUsage{
					Prompt:     resp.UsageMetadata.PromptTokenCount,
					Completion: resp.UsageMetadata.CandidatesTokenCount,
					Total:      resp.UsageMetadata.TotalTokenCount,
				}
			}
			return &ConversationResult{Response: "", Usage: tokenUsage}, nil
		}

		candidate := resp.Candidates[0]
		modelContent := candidate.Content
		functionCalls := make([]GeminiFunctionCall, 0)
		textParts := make([]string, 0)
		for _, part := range modelContent.Parts {
			if part.FunctionCall != nil {
				functionCalls = append(functionCalls, *part.FunctionCall)
				continue
			}

			if strings.TrimSpace(part.Text) != "" {
				textParts = append(textParts, part.Text)
			}
		}

		var tokenUsage *TokenUsage
		if resp.UsageMetadata != nil {
			tokenUsage = &TokenUsage{
				Prompt:     resp.UsageMetadata.PromptTokenCount,
				Completion: resp.UsageMetadata.CandidatesTokenCount,
				Total:      resp.UsageMetadata.TotalTokenCount,
			}
		}

		if len(functionCalls) == 0 {
			return &ConversationResult{Response: strings.Join(textParts, "\n"), Usage: tokenUsage}, nil
		}

		contents = append(contents, modelContent)
		responseParts := make([]GeminiPart, 0, len(functionCalls))
		for _, functionCall := range functionCalls {
			toolOutput, err := agent.callTool(ctx, functionCall.Name, functionCall.Args)
			if err != nil {
				return nil, err
			}
			responseParts = append(responseParts, GeminiPart{
				FunctionResponse: &GeminiFunctionResponse{
					ID:       functionCall.ID,
					Name:     functionCall.Name,
					Response: map[string]interface{}{"result": toolOutput},
				},
			})
		}
		contents = append(contents, GeminiContent{
			Role:  "user",
			Parts: responseParts,
		})
		req.Contents = contents
	}

	return nil, fmt.Errorf("tool loop exceeded maximum iterations")
}

func (agent *LLMAgent) runOpenAI(ctx context.Context, messages []AgentConversationMessage, maxTokens int, openAITools []OpenAITool) (*ConversationResult, error) {
	openAIMessages := make([]OpenAIMessage, 0, len(messages))
	for _, input := range messages {
		if strings.TrimSpace(input.Message) == "" {
			continue
		}

		role := strings.ToLower(strings.TrimSpace(input.Role))
		if func() bool {
			for _, v := range []string{"system", "assistant", "user", "tool"} {
				if role == v {
					return true
				}
			}
			return false
		}() {
			role = "user"
		}

		openAIMessages = append(openAIMessages, OpenAIMessage{Role: role, Content: input.Message})
	}

	if len(openAIMessages) == 0 {
		return nil, fmt.Errorf("at least one non-empty message is required")
	}

	req := OpenAIChatCompletionRequest{
		Model:     agent.modelName,
		Messages:  openAIMessages,
		MaxTokens: maxTokens,
	}

	if len(openAITools) > 0 {
		req.Tools = openAITools
		req.ToolChoice = "auto"
	}

	resp, err := agent.callOpenAI(ctx, req)
	if err != nil {
		return nil, err
	}

	var tokenUsage *TokenUsage
	if resp.Usage != nil {
		tokenUsage = &TokenUsage{
			Prompt:     resp.Usage.PromptTokens,
			Completion: resp.Usage.CompletionTokens,
			Total:      resp.Usage.TotalTokens,
		}
	}

	if len(resp.Choices) == 0 {
		return &ConversationResult{Response: "", Usage: tokenUsage}, nil
	}

	msg := resp.Choices[0].Message

	for _, toolCall := range msg.ToolCalls {
		if toolCall.Type != "function" && strings.TrimSpace(toolCall.Type) != "" {
			continue
		}

		argsMap := map[string]interface{}{}
		if strings.TrimSpace(toolCall.Function.Arguments) != "" {
			if unmarshalErr := json.Unmarshal([]byte(toolCall.Function.Arguments), &argsMap); unmarshalErr != nil {
				return nil, fmt.Errorf("failed to parse OpenAI tool arguments: %w", unmarshalErr)
			}
		}

		toolResult, err := agent.callTool(ctx, toolCall.Function.Name, argsMap)
		if err != nil {
			return nil, err
		}
		return &ConversationResult{Response: toolResult, Usage: tokenUsage}, nil
	}

	return &ConversationResult{Response: msg.Content, Usage: tokenUsage}, nil
}

func (agent *LLMAgent) callGemini(ctx context.Context, req GeminiGenerateContentRequest) (*GeminiGenerateContentResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Gemini request: %w", err)
	}

	endpoint := strings.TrimRight(agent.providerBaseUrl, "/") + "/models/" + url.PathEscape(agent.modelName) + ":generateContent?key=" + url.QueryEscape(agent.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := agent.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call Gemini API: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Gemini response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini API error: %s", string(body))
	}

	var response GeminiGenerateContentResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini response: %w", err)
	}

	return &response, nil
}

func (agent *LLMAgent) initializeMCPClient(ctx context.Context) error {
	parsedURL, err := url.Parse(agent.serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	if strings.TrimSpace(parsedURL.Scheme) == "" || strings.TrimSpace(parsedURL.Host) == "" {
		return fmt.Errorf("server URL must include scheme and host, e.g. http://127.0.0.1:8080/mcp")
	}

	endpoint := parsedURL.Path
	if strings.TrimSpace(endpoint) == "" {
		endpoint = "/mcp"
	}

	baseURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
	transport := mcp_http_transport.NewHTTPClientTransport(endpoint).WithBaseURL(baseURL)
	agent.client = mcp_golang.NewClient(transport)

	initResp, err := agent.client.Initialize(ctx)
	if err != nil {
		return err
	}

	if initResp != nil && initResp.Instructions != nil && strings.TrimSpace(*initResp.Instructions) != "" {
		agent.mcpInstructions = initResp.Instructions
	}

	return nil
}

// callClaude calls the Claude API
func (agent *LLMAgent) callClaude(ctx context.Context, req ClaudeRequest) (*ClaudeResponse, error) {
	if strings.TrimSpace(agent.apiKey) == "" {
		return nil, fmt.Errorf("anthropic_api_key config is not set")
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", agent.providerBaseUrl+"/messages", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", agent.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := agent.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call Claude: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude API error: %s", string(body))
	}

	var response ClaudeResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != nil {
		return nil, fmt.Errorf("claude error: %s", response.Error.Message)
	}

	return &response, nil
}

func (agent *LLMAgent) callOpenAI(ctx context.Context, req OpenAIChatCompletionRequest) (*OpenAIChatCompletionResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI request: %w", err)
	}

	endpoint := agent.providerBaseUrl + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI request: %w", err)
	}

	if strings.TrimSpace(agent.apiKey) == "" {
		return nil, fmt.Errorf("api key config is not set")
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+agent.apiKey)

	httpResp, err := agent.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI API: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read OpenAI response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API error: %s", string(body))
	}

	var response OpenAIChatCompletionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}
	if response.Error != nil {
		return nil, fmt.Errorf("OpenAI error: %s", response.Error.Message)
	}

	return &response, nil
}

// callTool calls a tool on the MCP server
func (agent *LLMAgent) callTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	result, err := agent.client.CallTool(ctx, toolName, args)
	if err != nil {
		return "", fmt.Errorf("failed to call tool %s: %w", toolName, err)
	}

	if result == nil || len(result.Content) == 0 {
		return "", nil
	}

	output := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		if content == nil {
			continue
		}
		if content.TextContent != nil {
			output = append(output, content.TextContent.Text)
			continue
		}
		fallback, marshalErr := json.Marshal(content)
		if marshalErr == nil {
			output = append(output, string(fallback))
		}
	}

	if len(output) == 0 {
		return "", nil
	}

	return strings.Join(output, "\n"), nil
}
