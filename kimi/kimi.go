package kimi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"focus-ai/config"
	"io"
	"net/http"
)

const ROLE_SYSTEM = "system"
const ROLE_USER = "user"
const ROLE_ASSISTANT = "assistant"

var ErrEmptyPrompt = errors.New("prompt is empty")

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type APIRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature"`
}

type APIResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
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

var blogSummaryPrompt = `你是一个擅长对博客和新闻做总结的助手，我将会提供给你博客或者新闻的链接，你需要对此进行总结。
总结的时候你需要注意以下几点：
1. 不论链接里的内容是什么语言的，总结的结果都必须是中文的。
2. 总结的格式是：
标题：这里给出中文的标题
内容总结：
这里写总结的内容。
3. 总结的时候要尽可能地详细。`

var BaseBlogSummaryPrompt = Message{Role: ROLE_SYSTEM, Content: blogSummaryPrompt}

func SendChatRequest(prompt string) (result string, err error) {
	if prompt == "" {
		return "", ErrEmptyPrompt
	}

	url := "https://api.moonshot.cn/v1/chat/completions"
	requestBody, _ := json.Marshal(APIRequest{
		Model: config.AI.KimiModel,
		Messages: []Message{
			BaseBlogSummaryPrompt,
			{Role: ROLE_USER, Content: prompt},
		},
		Temperature: 0.3,
	})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(requestBody))
	if err != nil {
		return
	}

	req.Header.Add("Authorization", "Bearer "+config.AI.KimiSecretKey)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("statusCode %d, request error: %v", resp.StatusCode, string(body))
		return
	}

	respData := APIResponse{}
	err = json.Unmarshal(body, &respData)
	if err != nil {
		return
	}

	if len(respData.Choices) == 0 {
		return
	}

	result = respData.Choices[0].Message.Content
	return
}
