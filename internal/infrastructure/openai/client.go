package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rakibulbh/safe-london/internal/domain"
)

const defaultBaseURL = "https://api.openai.com/v1/chat/completions"

const systemPrompt = `You are an expert London Met Police Crime Analyst. You will receive an image of a situation and a text summary of recent crimes in the exact same location. Your job is to: 1. Verify if the image shows a real, active threat. 2. Analyze the historical text to determine what the likely trend is and where suspects typically head next. Return ONLY valid JSON matching this schema: { "is_threat": boolean, "threat_level": 1-5, "description": "string explaining the image", "trend_analysis": "string explaining the historical pattern and likely escape route based on the text provided" }`

type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		http:    http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type chatPayload struct {
	Model     string          `json:"model"`
	Messages  []chatMessage   `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
}

type chatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type chatCompletionResponse struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Message choiceMessage `json:"message"`
}

type choiceMessage struct {
	Content string `json:"content"`
}

func encodeImageToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func buildPayload(imageData []byte, historicalContext string) chatPayload {
	b64 := encodeImageToBase64(imageData)
	textContent := fmt.Sprintf("HISTORICAL CONTEXT:\n%s\n\nANALYZE THE IMAGE ABOVE IN CONTEXT OF THE HISTORY.", historicalContext)

	userContent := []map[string]interface{}{
		{"type": "text", "text": textContent},
		{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + b64}},
	}

	return chatPayload{
		Model: "gpt-4o",
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
		MaxTokens: 300,
	}
}

// Analyze sends the image and historical context to OpenAI for threat analysis.
func (c *Client) Analyze(ctx context.Context, imageData []byte, historicalContext string) (*domain.LLMAnalysisResult, error) {
	payload := buildPayload(imageData, historicalContext)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai api returned %d", resp.StatusCode)
	}

	var completionResp chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completionResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(completionResp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	raw := strings.TrimSpace(completionResp.Choices[0].Message.Content)
	raw = stripMarkdownFences(raw)

	var result domain.LLMAnalysisResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse llm json %q: %w", raw, err)
	}
	return &result, nil
}

// stripMarkdownFences handles LLMs that wrap JSON in ```json ... ``` blocks.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s[3:], "\n"); idx != -1 {
			s = s[3+idx+1:]
		}
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
	}
	return strings.TrimSpace(s)
}
