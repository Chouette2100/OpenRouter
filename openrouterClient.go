package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type openRouterResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Created int64  `json:"created"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func buildMessages(systemPrompt string, history []QaRecord, question string) []map[string]string {
	messages := make([]map[string]string, 0, len(history)*2+2)
	if systemPrompt != "" {
		messages = append(messages, map[string]string{"role": "system", "content": systemPrompt})
	}
	for _, rec := range history {
		if rec.Question != "" {
			messages = append(messages, map[string]string{"role": "user", "content": rec.Question})
		}
		if rec.Answer != "" {
			messages = append(messages, map[string]string{"role": "assistant", "content": rec.Answer})
		}
	}
	messages = append(messages, map[string]string{"role": "user", "content": question})
	return messages
}

func askOpenRouter(apiKey, model, systemPrompt, question string, history []QaRecord, maxTokens int, temperature float64) (string, int, int, int, error) {
	if apiKey == "" {
		return "", 0, 0, 0, fmt.Errorf("OPENROUTER_API_KEY is empty")
	}
	if model == "" {
		model = defaultModelName()
	}
	payload := map[string]interface{}{
		"model":       model,
		"messages":    buildMessages(systemPrompt, history, question),
		"max_tokens":  maxTokens,
		"temperature": temperature,
	}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return "", 0, 0, 0, err
	}

	req, err := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", 0, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "http://localhost")
	req.Header.Set("X-Title", "OpenRouter Chat")

	resp, err := (&http.Client{Timeout: 180 * time.Second}).Do(req)
	if err != nil {
		return "", 0, 0, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, 0, 0, fmt.Errorf("OpenRouter API Error %d: %s", resp.StatusCode, string(body))
	}
	var result openRouterResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", 0, 0, 0, err
	}
	if result.Error != nil && result.Error.Message != "" {
		return "", 0, 0, 0, fmt.Errorf("OpenRouter: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", 0, 0, 0, fmt.Errorf("OpenRouter returned no choices")
	}
	answer := result.Choices[0].Message.Content
	inputTokens := result.Usage.PromptTokens
	outputTokens := result.Usage.CompletionTokens
	totalTokens := result.Usage.TotalTokens

	log.Println("========================================")
	log.Println("OpenRouter API response")
	log.Println("========================================")
	log.Printf("応答生成時刻: %s\n", time.Unix(result.Created, 0).Format(time.RFC3339))
	log.Printf("モデル: %s\n", result.Model)
	log.Printf("応答ID: %s\n", result.ID)
	log.Printf("入力トークン: %d\n", result.Usage.PromptTokens)
	log.Printf("出力トークン: %d\n", result.Usage.CompletionTokens)
	log.Printf("合計トークン: %d\n", result.Usage.TotalTokens)
	log.Println("----------------------------------------")
	log.Println("回答:")
	log.Println(answer)
	log.Println("----------------------------------------")

	return answer, inputTokens, outputTokens, totalTokens, nil
}
