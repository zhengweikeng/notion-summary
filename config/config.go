package config

import (
	"os"
)

type NotionConf struct {
	NotionApiKey  string
	NotionDBID    string
	NotionVersion string
}

type AIConf struct {
	KimiSecretKey string
	KimiModel     string
}

type EmailConf struct {
	APIKey string
	FROM   string
	To     string
}

var Notion NotionConf
var AI AIConf
var Email EmailConf

func InitConfig() {
	Notion = NotionConf{
		NotionApiKey:  getEnv("NOTION_API_KEY", ""),
		NotionDBID:    getEnv("NOTION_DATABASE_ID", ""),
		NotionVersion: getEnv("NOTION_VERSION", "2022-06-28"),
	}

	AI = AIConf{
		KimiSecretKey: getEnv("MOONSHOT_API_KEY", ""),
		KimiModel:     getEnv("KIMI_MODEL", "moonshot-v1-8k"),
	}

	Email = EmailConf{
		APIKey: getEnv("RESEND_API_KEY", ""),
		FROM:   getEnv("RESEND_FROM", "onboarding@resend.dev"),
		To:     getEnv("RESEND_To", ""),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
