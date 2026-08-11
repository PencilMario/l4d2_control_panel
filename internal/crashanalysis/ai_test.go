package crashanalysis

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOpenAIClientNormalizesEndpointSendsModelAndRetriesServerFailure(t *testing.T) {
	var requests atomic.Int32
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer api-key" {
			t.Fatalf("request path=%s auth=%s", r.URL.Path, r.Header.Get("Authorization"))
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if requests.Load() == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"analysis result"}}]}`))
	}))
	defer server.Close()
	client, err := NewOpenAIClient(OpenAIConfig{Endpoint: server.URL + "/v1", Model: "local-model", APIKey: "api-key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Analyze(context.Background(), []byte("diagnostic input"))
	if err != nil || result != "analysis result" || requests.Load() != 2 {
		t.Fatalf("result=%q err=%v requests=%d body=%v", result, err, requests.Load(), body)
	}
	if body["model"] != "local-model" {
		t.Fatalf("body=%v", body)
	}
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) == 0 {
		t.Fatalf("messages=%v", body["messages"])
	}
	systemMessage, ok := messages[0].(map[string]any)
	if !ok || systemMessage["role"] != "system" {
		t.Fatalf("system message=%v", messages[0])
	}
	systemContent, ok := systemMessage["content"].(string)
	if !ok || !strings.Contains(systemContent, "Simplified Chinese") {
		t.Fatalf("system prompt must require Simplified Chinese: %q", systemContent)
	}
}

func TestOpenAIClientRejectsRemoteHTTPAndCapsResponse(t *testing.T) {
	if _, err := NewOpenAIClient(OpenAIConfig{Endpoint: "http://example.com/v1", Model: "model"}); err == nil {
		t.Fatal("accepted remote HTTP endpoint")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + strings.Repeat("x", 1025) + `"}}]}`))
	}))
	defer server.Close()
	client, err := NewOpenAIClient(OpenAIConfig{Endpoint: server.URL, Model: "model", MaxResponseBytes: 1024, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Analyze(context.Background(), []byte("input")); err == nil {
		t.Fatal("accepted oversized response")
	}
}
