package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEncodeImageToBase64(t *testing.T) {
	data := []byte("fake-image-data")
	b64 := encodeImageToBase64(data)
	if b64 == "" {
		t.Error("expected non-empty base64 string")
	}
	if len(b64) < len(data) {
		t.Error("base64 should be longer than raw bytes")
	}
}

func TestBuildPayload_Structure(t *testing.T) {
	imageData := []byte("test-image")
	history := "On 2024-01 at Oxford Street, robbery occurred."

	payload := buildPayload(imageData, history)

	if payload.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", payload.Model)
	}
	if payload.MaxTokens != 300 {
		t.Errorf("expected max_tokens 300, got %d", payload.MaxTokens)
	}
	if len(payload.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(payload.Messages))
	}
	if payload.Messages[0].Role != "system" {
		t.Error("first message should be system")
	}
	if payload.Messages[1].Role != "user" {
		t.Error("second message should be user")
	}

	userContent := payload.Messages[1].Content
	contentParts, ok := userContent.([]map[string]interface{})
	if !ok {
		t.Fatalf("expected user content to be []map, got %T", userContent)
	}
	if len(contentParts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(contentParts))
	}

	textPart := contentParts[0]
	if textPart["type"] != "text" {
		t.Errorf("first part should be text, got %v", textPart["type"])
	}
	text, _ := textPart["text"].(string)
	if text == "" {
		t.Error("text content should not be empty")
	}

	imagePart := contentParts[1]
	if imagePart["type"] != "image_url" {
		t.Errorf("second part should be image_url, got %v", imagePart["type"])
	}
}

func TestAnalyze_SuccessfulResponse(t *testing.T) {
	llmResult := map[string]interface{}{
		"is_threat":      true,
		"threat_level":   4,
		"description":    "Active moped theft in progress",
		"trend_analysis": "Suspects typically flee north",
	}
	resultJSON, _ := json.Marshal(llmResult)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("expected Authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected Content-Type application/json")
		}

		resp := chatCompletionResponse{
			Choices: []choice{
				{Message: choiceMessage{Content: string(resultJSON)}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))
	result, err := client.Analyze(context.Background(), []byte("fake-image"), "historical context")
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if !result.IsThreat {
		t.Error("expected is_threat=true")
	}
	if result.ThreatLevel != 4 {
		t.Errorf("expected threat_level=4, got %d", result.ThreatLevel)
	}
	if result.Description != "Active moped theft in progress" {
		t.Errorf("unexpected description: %s", result.Description)
	}
	if result.TrendAnalysis != "Suspects typically flee north" {
		t.Errorf("unexpected trend_analysis: %s", result.TrendAnalysis)
	}
}

func TestAnalyze_MalformedLLMResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{
			Choices: []choice{
				{Message: choiceMessage{Content: "not valid json at all"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))
	_, err := client.Analyze(context.Background(), []byte("fake-image"), "context")
	if err == nil {
		t.Error("expected error for malformed LLM JSON response")
	}
}

func TestAnalyze_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{Choices: []choice{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))
	_, err := client.Analyze(context.Background(), []byte("fake-image"), "context")
	if err == nil {
		t.Error("expected error for empty choices")
	}
}

func TestAnalyze_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))
	_, err := client.Analyze(context.Background(), []byte("fake-image"), "context")
	if err == nil {
		t.Error("expected error for API 500")
	}
}

func TestAnalyze_LLMReturnsJSONWithMarkdownFencing(t *testing.T) {
	llmResult := map[string]interface{}{
		"is_threat":      false,
		"threat_level":   1,
		"description":    "No threat detected",
		"trend_analysis": "Area is quiet",
	}
	resultJSON, _ := json.Marshal(llmResult)
	// LLMs sometimes wrap JSON in markdown code fences
	fenced := "```json\n" + string(resultJSON) + "\n```"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{
			Choices: []choice{
				{Message: choiceMessage{Content: fenced}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))
	result, err := client.Analyze(context.Background(), []byte("fake-image"), "context")
	if err != nil {
		t.Fatalf("should handle markdown-fenced JSON: %v", err)
	}
	if result.ThreatLevel != 1 {
		t.Errorf("expected threat_level=1, got %d", result.ThreatLevel)
	}
}
