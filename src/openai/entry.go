package openai

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"getaced.io/src/config"
	"getaced.io/src/structs"

	"golang.org/x/net/html"
)

const baseSystemPrompt = `You are a high school student answering questions from a worksheet or assignment.

Rules:
- If multiple choice, just give the letter (A, B, C, D, etc.)
- If open ended, answer short and simple like a student would. casual, not formal. like you understood it but your not trying to impress anyone.
- Do not explain your reasoning unless the question asks you to
- Do not add anything extra
- Do not skip any questions

Return your answers as JSON in this exact format: {"answers": [{"number": 1, "answer": "A"}, {"number": 2, "answer": "the answer text"}]}`

type Client struct {
	APIKey string
	HTTP   *http.Client
	Model  string
}

func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = config.DefaultOpenAIModel
	}
	return &Client{
		APIKey: apiKey,
		HTTP:   &http.Client{Timeout: config.HTTPTimeoutOpenAI},
		Model:  model,
	}
}

type chatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	ResponseFormat *respFormat   `json:"response_format,omitempty"`
}

type respFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func buildSystemPrompt(customPrompt string) string {
	prompt := baseSystemPrompt
	if customPrompt != "" {
		prompt += "\n\nAdditional instructions: " + customPrompt
	}
	return prompt
}

func (c *Client) doChat(messages []chatMessage) (*structs.AnswerResponse, error) {
	reqBody := chatRequest{
		Model:          c.Model,
		Messages:       messages,
		ResponseFormat: &respFormat{Type: "json_object"},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	var answers structs.AnswerResponse
	if err := json.Unmarshal([]byte(chatResp.Choices[0].Message.Content), &answers); err != nil {
		return nil, fmt.Errorf("unmarshal answers: %w", err)
	}

	return &answers, nil
}

func (c *Client) ProcessText(text string, customPrompt string) (*structs.AnswerResponse, error) {
	messages := []chatMessage{
		{Role: "system", Content: buildSystemPrompt(customPrompt)},
		{Role: "user", Content: text},
	}
	return c.doChat(messages)
}

func (c *Client) ProcessImage(imageBytes []byte, customPrompt string) (*structs.AnswerResponse, error) {
	b64 := base64.StdEncoding.EncodeToString(imageBytes)
	imageURL := "data:image/png;base64," + b64

	content := []map[string]interface{}{
		{
			"type": "text",
			"text": "Answer all questions visible in this image.",
		},
		{
			"type": "image_url",
			"image_url": map[string]string{
				"url": imageURL,
			},
		},
	}

	messages := []chatMessage{
		{Role: "system", Content: buildSystemPrompt(customPrompt)},
		{Role: "user", Content: content},
	}
	return c.doChat(messages)
}

func StripHTML(htmlContent []byte) string {
	tokenizer := html.NewTokenizer(bytes.NewReader(htmlContent))
	var sb strings.Builder
	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return strings.TrimSpace(sb.String())
		case html.TextToken:
			text := strings.TrimSpace(tokenizer.Token().Data)
			if text != "" {
				sb.WriteString(text)
				sb.WriteString(" ")
			}
		case html.StartTagToken:
			tn, _ := tokenizer.TagName()
			tag := string(tn)
			if tag == "script" || tag == "style" {
				for {
					tt := tokenizer.Next()
					if tt == html.EndTagToken {
						break
					}
					if tt == html.ErrorToken {
						return strings.TrimSpace(sb.String())
					}
				}
			}
		}
	}
}
