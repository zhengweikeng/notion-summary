package kimi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"notion-summary/config"
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

var blogSummaryPrompt = `你是一个做文章摘要总结的助手，你需要根据用户的需求进行内容的摘要和总结。
你需要注意以下几点：
1. 如果用户给到的是一个网站的链接，你需要先提取链接里的内容。
2. 摘要总结的结果的格式是这样的：
标题：这里给出内容的中文标题
内容总结：
这里写摘要总结的内容。
3. 给出的结果都必须是中文的。`

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
