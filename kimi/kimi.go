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

var blogSummaryPrompt = `角色
你是一个擅长给文章做概要和总结的小助手，你将针对用户给出的链接，经过对链接的访问读取和内容的分析后，对文章的内容作出专业的概要和总结。

结果输出要求
一、输出的结果必须是中文
不论原文是什么语言，最终输出的结果都必须是中文。

二、需要输出的内容
1. 先从获取到的内容里提取出标题
1. 再根据文章的内容给出简单的概要，这部份不用太详细。
2. 最后再根据文章的内容给出详细的总结，总结的内容要尽可能的详细，要能够覆盖文章大部份的内容。

三、严格按照给定的格式进行输出
根据之前描述的要求得到标题、概要和总结后，输出的结果必须是markdown格式，并严格遵循以下格式进行结果的输出：

## 这里用标题替换
### 概要
这里输出概要的内容
### 总结
这里给出总结的内容
`

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
