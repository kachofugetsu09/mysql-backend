package deepseek

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"mysql-agent/config"
)

type Client struct {
	APIKey          string
	BaseURL         string
	Model           string
	client          *http.Client
	analysisTimeout time.Duration
}

func NewClient() *Client {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	keySource := "env"
	if apiKey == "" && config.AppConfig != nil {
		apiKey = config.AppConfig.DeepSeek.APIKey
		keySource = "config"
	}
	if apiKey == "" {
		log.Printf("[DeepSeek] warning: API key not configured")
	}

	baseURL := "https://api.deepseek.com"
	model := "deepseek-chat"
	timeout := 120 * time.Second
	analysisTimeout := 120 * time.Second

	if config.AppConfig != nil {
		if config.AppConfig.DeepSeek.BaseURL != "" {
			baseURL = config.AppConfig.DeepSeek.BaseURL
		}
		if config.AppConfig.DeepSeek.Model != "" {
			model = config.AppConfig.DeepSeek.Model
		}
		if config.AppConfig.DeepSeek.Timeout > 0 {
			timeout = config.AppConfig.DeepSeek.Timeout
		}
		if config.AppConfig.DeepSeek.AnalysisTimeout > 0 {
			analysisTimeout = config.AppConfig.DeepSeek.AnalysisTimeout
		} else {
			analysisTimeout = timeout
		}
	}

	log.Printf("[DeepSeek] init model=%s endpoint=%s timeout=%s analysis_timeout=%s key_source=%s",
		model, baseURL, timeout, analysisTimeout, keySource)

	return &Client{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
		client: &http.Client{
			Timeout: timeout,
		},
		analysisTimeout: analysisTimeout,
	}
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Function struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (c *Client) ChatWithBody(jsonData []byte) (*ChatResponse, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("DeepSeek API key is not configured")
	}

	start := time.Now()
	url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	httpClient := c.client
	if c.analysisTimeout > 0 && httpClient.Timeout != c.analysisTimeout {
		clientCopy := *httpClient
		clientCopy.Timeout = c.analysisTimeout
		httpClient = &clientCopy
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[DeepSeek] http_error err=%v", err)
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[DeepSeek] api_error status=%d body=%s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	log.Printf("[DeepSeek] chat duration=%s choices=%d", time.Since(start), len(chatResp.Choices))
	return &chatResp, nil
}
