package api

import (
	"fmt"
	"net/http"
)

type PageCreateRequest struct {
	Parent     Parent              `json:"parent,omitempty"`
	Properties map[string]Property `json:"properties,omitempty"`
	Children   []Block             `json:"children,omitempty"`
}

type PageCreateResponse struct {
	Object string `json:"object,omitempty"`
	ID     string `json:"id,omitempty"`
}

type PageUpdateRequest struct {
	Properties map[string]Property `json:"properties,omitempty"`
}

type PageUpdateResponse struct {
	Object string `json:"object,omitempty"`
	ID     string `json:"id,omitempty"`
}

func CreatePageInDatabase(databaseID string,
	properties map[string]Property,
	children []Block) (string, error) {
	url := "https://api.notion.com/v1/pages"
	reqBody := PageCreateRequest{
		Parent:     Parent{DatabaseID: databaseID},
		Properties: properties,
		Children:   children,
	}
	page := &PageCreateResponse{}

	err := makeRequest(http.MethodPost, url, reqBody, page)
	if err != nil {
		return "", err
	}

	return page.ID, nil
}

func UpdatePage(pageID string, properties map[string]Property) error {
	url := fmt.Sprintf("https://api.notion.com/v1/pages/%s", pageID)
	reqBody := PageUpdateRequest{
		Properties: properties,
	}
	page := &PageUpdateResponse{}

	return makeRequest(http.MethodPatch, url, reqBody, page)
}
