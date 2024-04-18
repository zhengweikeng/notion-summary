package api

import (
	"fmt"
	"net/http"
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
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Title    []TitleProperty    `json:"title,omitempty"`
	Checkbox bool               `json:"checkbox,omitempty"`
	URL      string             `json:"url,omitempty"`
	Select   *SelectProperty    `json:"select,omitempty"`
	Date     *DateProperty      `json:"date,omitempty"`
	RichText []RichTextProperty `json:"rich_text,omitempty"`
}

type TitleProperty struct {
	Type        string    `json:"type,omitempty"`
	Text        TextField `json:"text,omitempty"`
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

type SelectProperty struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Color string `json:"color,omitempty"`
}

type DateProperty struct {
	Start    string `json:"start,omitempty"`
	End      string `json:"end,omitempty"`
	TimeZone string `json:"time_zone,omitempty"`
}

type RichTextProperty struct {
	Type        string    `json:"type,omitempty"`
	Text        TextField `json:"text,omitempty"`
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

type TextField struct {
	Content string `json:"content,omitempty"`
	Link    string `json:"link,omitempty"`
}

type Parent struct {
	PageID     string `json:"page_id,omitempty"`
	DatabaseID string `json:"database_id,omitempty"`
}

func FetchDatabaseItems(databaseID string, filter *DatabaseFilter) (dbItems []DatabaseItem, err error) {
	url := fmt.Sprintf("https://api.notion.com/v1/databases/%s/query", databaseID)
	reqBody := DatabaseRequestBody{Filter: *filter}
	database := &DatabaseResponse{}

	err = makeRequest(http.MethodPost, url, reqBody, database)
	if err != nil {
		return nil, err
	}

	return database.Results, nil
}
