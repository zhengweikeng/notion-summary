package notion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"focus-ai/config"
	"io"
	"log"
	"net/http"
)

type ItemCategory string

const (
	BLOG ItemCategory = "Blog"
)

// database模板字段
const (
	TEMP_PROP_NAME     = "Name"     // 博客名字
	TEMP_PROP_RSS      = "RSS"      // RSS链接
	TEMP_PROP_IS_STSR  = "Star"     // 是否重点关注
	TEMP_PROP_CATEGORY = "Category" // 数据类别，Blog | News
)

type DatabaseRequestBody struct {
	Filter DatabaseFilter `json:"filter,omitempty"`
}

type DatabaseFilter struct {
	Property string                 `json:"property,omitempty"`
	Select   map[string]interface{} `json:"select,omitempty"`
}

type DatabaseResponse struct {
	Object     string         `json:"object"`
	Results    []DatabaseItem `json:"results"`
	NextCursor string         `json:"next_cursor,omitempty"`
	HasMore    bool           `json:"has_more"`
}

// DatabaseItem represents a single record in a Notion database.
type DatabaseItem struct {
	Object         string              `json:"object"`
	ID             string              `json:"id"`
	CreatedTime    string              `json:"created_time"`
	LastEditedTime string              `json:"last_edited_time"`
	Properties     map[string]Property `json:"properties"`
}

type Property struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Title    []TitleProperty `json:"title"`
	Checkbox bool            `json:"checkbox"`
	URL      string          `json:"url"`
	Select   SelectProperty  `json:"select"`
}

type TitleProperty struct {
	Type string `json:"type,omitempty"`
	Text struct {
		Content string `json:"content,omitempty"`
		Link    string `json:"link,omitempty"`
	} `json:"text,omitempty"`
	Annotations struct {
		Bold          bool   `json:"bold,omitempty"`
		Italic        bool   `json:"italic,omitempty"`
		Strikethrough bool   `json:"strikethrough,omitempty"`
		Underline     bool   `json:"underline,omitempty"`
		Code          bool   `json:"code,omitempty"`
		Color         string `json:"color,omitempty"`
	} `json:"annotations,omitempty"`
	PlainText string `json:"plain_text,omitempty"`
	Href      string `json:"href,omitempty"`
}

// TextProperty represents the specifics of a text property, which could be different based on the property type.
type TextProperty struct {
	Content string `json:"content"`
}

type SelectProperty struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Color string `json:"color,omitempty"`
}

func fetchDatabase(category ItemCategory) (dbItems []DatabaseItem, err error) {
	url := fmt.Sprintf("https://api.notion.com/v1/databases/%s/query", config.Notion.NotionDBID)
	var requestBody []byte
	if category != "" {
		requestBody, err = json.Marshal(DatabaseRequestBody{
			Filter: DatabaseFilter{
				Property: TEMP_PROP_CATEGORY,
				Select: map[string]interface{}{
					"equals": category,
				},
			},
		})
		if err != nil {
			return
		}
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(requestBody))
	if err != nil {
		return
	}

	req.Header.Add("Authorization", "Bearer "+config.Notion.NotionApiKey)
	req.Header.Add("Notion-Version", config.Notion.NotionVersion)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error making request:", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error reading response:", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("got error:%s", string(body))
		return
	}

	database := DatabaseResponse{}
	err = json.Unmarshal(body, &database)
	if err != nil {
		log.Printf("parse body %s error:%v\n", string(body), err)
		return
	}

	return database.Results, nil
}
