package api

import (
	"fmt"
	"net/http"
)

type BlockChildResponse struct {
	Object     string  `json:"object,omitempty"`
	Results    []Block `json:"results,omitempty"`
	NextCursor string  `json:"next_cursor,omitempty"`
	HasMore    bool    `json:"has_more,omitempty"`
}

type Block struct {
	Object    string          `json:"object,omitempty"`
	ID        string          `json:"id,omitempty"`
	Type      string          `json:"type,omitempty"`
	Bookmark  *BlockBookmark  `json:"bookmark,omitempty"`
	Heading2  *BlockHeading2  `json:"heading_2,omitempty"`
	Paragraph *BlockParagraph `json:"paragraph,omitempty"`
}

type BlockBookmark struct {
	Caption []RichTextProperty `json:"caption,omitempty"`
	URL     string             `json:"url,omitempty"`
}

type BlockHeading2 struct {
	RichText []RichTextProperty `json:"rich_text,omitempty"`
}

type BlockParagraph struct {
	RichText []RichTextProperty `json:"rich_text,omitempty"`
}

func FetchBlockChilds(blockID string) ([]Block, error) {
	url := fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children", blockID)
	blockChild := &BlockChildResponse{}

	err := makeRequest(http.MethodGet, url, nil, blockChild)
	if err != nil {
		return nil, err
	}

	return blockChild.Results, nil
}
