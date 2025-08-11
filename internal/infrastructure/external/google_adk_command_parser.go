package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/personal/home-work-ad-process/internal/domain/command"
)

// GoogleADKCommandParser implements command.Parser using Google ADK
type GoogleADKCommandParser struct {
	apiKey    string
	endpoint  string
	client    *http.Client
	fallback  *MockCommandParser // Fallback to mock parser
}

// GoogleADKRequest represents the request to Google AI Studio API
type GoogleADKRequest struct {
	Contents []Content `json:"contents"`
	GenerationConfig GenerationConfig `json:"generationConfig"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type GenerationConfig struct {
	Temperature     float32 `json:"temperature"`
	TopK           int     `json:"topK"`
	TopP           float32 `json:"topP"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

// GoogleADKResponse represents the response from Google AI Studio API
type GoogleADKResponse struct {
	Candidates []Candidate `json:"candidates"`
	Error      *APIError   `json:"error,omitempty"`
}

type Candidate struct {
	Content Content `json:"content"`
	FinishReason string `json:"finishReason"`
}

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// NewGoogleADKCommandParser creates a new Google ADK command parser
func NewGoogleADKCommandParser(apiKey string) *GoogleADKCommandParser {
	return &GoogleADKCommandParser{
		apiKey:   apiKey,
		endpoint: "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash-latest:generateContent",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		fallback: NewMockCommandParser(),
	}
}

// Parse parses natural language text into a structured command using Google ADK
func (p *GoogleADKCommandParser) Parse(ctx context.Context, text string) (*command.Command, error) {
	// First try Google ADK API
	cmd, err := p.parseWithGoogleADK(ctx, text)
	if err != nil {
		// Fallback to mock parser if Google ADK fails
		return p.fallback.Parse(ctx, text)
	}
	
	return cmd, nil
}

// parseWithGoogleADK processes command using Google AI Studio API
func (p *GoogleADKCommandParser) parseWithGoogleADK(ctx context.Context, text string) (*command.Command, error) {
	prompt := p.buildCommandParsingPrompt(text)
	
	request := GoogleADKRequest{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: GenerationConfig{
			Temperature:     0.1,
			TopK:           40,
			TopP:           0.95,
			MaxOutputTokens: 1024,
		},
	}
	
	response, err := p.callGoogleAPI(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to call Google API: %w", err)
	}
	
	return p.parseAPIResponse(text, response)
}

// buildCommandParsingPrompt creates a detailed prompt for command parsing
func (p *GoogleADKCommandParser) buildCommandParsingPrompt(text string) string {
	return fmt.Sprintf(`You are an AI assistant that parses natural language commands for an ad processing queue system.

Parse the following command and return a JSON response with the command details:

Command: "%s"

Supported command types and their formats:

1. Queue Modification Commands:
   - "Change priority to {X} for all ads in the {gameFamily} family"
   - "Set priority to {X} for ads older than {Y} minutes"
   
2. System Configuration Commands:
   - "Enable starvation mode"
   - "Set maximum wait time to {X} seconds"
   - "Set worker count to {X}"
   
3. Status and Analytics Commands:
   - "Show the next {X} ads to be processed"
   - "List all ads waiting longer than {X} minutes"
   - "What's the current queue distribution by priority?"
   - "Show queue performance summary"

Return ONLY a JSON object in this format:
{
  "intent": "command_intent_name",
  "type": "queue_modification|system_configuration|status_query|analytics",
  "parameters": {
    "parameter_name": "parameter_value"
  },
  "valid": true,
  "error": null
}

If the command cannot be parsed, return:
{
  "intent": "unknown",
  "type": "unknown",
  "parameters": {},
  "valid": false,
  "error": "Unable to parse command"
}

Priority values must be between 1-5. Validate all numeric parameters.`, text)
}

// callGoogleAPI makes the HTTP request to Google AI Studio API
func (p *GoogleADKCommandParser) callGoogleAPI(ctx context.Context, request GoogleADKRequest) (*GoogleADKResponse, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	url := fmt.Sprintf("%s?key=%s", p.endpoint, p.apiKey)
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	
	var response GoogleADKResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	if response.Error != nil {
		return nil, fmt.Errorf("API error %d: %s", response.Error.Code, response.Error.Message)
	}
	
	return &response, nil
}

// parseAPIResponse converts Google API response to command structure
func (p *GoogleADKCommandParser) parseAPIResponse(originalText string, response *GoogleADKResponse) (*command.Command, error) {
	if len(response.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}
	
	candidate := response.Candidates[0]
	if len(candidate.Content.Parts) == 0 {
		return nil, fmt.Errorf("no content parts in response")
	}
	
	responseText := candidate.Content.Parts[0].Text
	
	// Parse JSON response from Google AI
	var parsedCommand struct {
		Intent     string                 `json:"intent"`
		Type       string                 `json:"type"`
		Parameters map[string]interface{} `json:"parameters"`
		Valid      bool                   `json:"valid"`
		Error      *string                `json:"error"`
	}
	
	if err := json.Unmarshal([]byte(responseText), &parsedCommand); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}
	
	if !parsedCommand.Valid {
		errorMsg := "unknown error"
		if parsedCommand.Error != nil {
			errorMsg = *parsedCommand.Error
		}
		return nil, fmt.Errorf("invalid command: %s", errorMsg)
	}
	
	// Convert string type to command type enum
	commandType := p.parseCommandType(parsedCommand.Type)
	
	return command.NewCommand(
		originalText,
		commandType,
		parsedCommand.Intent,
		parsedCommand.Parameters,
	), nil
}

// parseCommandType converts string type to command.CommandType
func (p *GoogleADKCommandParser) parseCommandType(typeStr string) command.CommandType {
	switch typeStr {
	case "queue_modification":
		return command.CommandTypeQueueModification
	case "system_configuration":
		return command.CommandTypeSystemConfiguration
	case "status_query":
		return command.CommandTypeStatusQuery
	case "analytics":
		return command.CommandTypeAnalytics
	default:
		return command.CommandTypeStatusQuery // Default fallback
	}
}

// ValidateCommand validates that a command has all required parameters
func (p *GoogleADKCommandParser) ValidateCommand(ctx context.Context, cmd *command.Command) error {
	// Use the same validation logic as the mock parser
	return p.fallback.ValidateCommand(ctx, cmd)
}