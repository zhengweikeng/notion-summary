package config

import (
	"os"
	"strconv"
)

type ServiceConf struct {
	Port             string
	BlogSyncInterval string
}

type NotionConf struct {
	NotionApiKey string
	NotionDBID   string
	SyncMaxSize  int
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

var Service ServiceConf
var Notion NotionConf
var AI AIConf
var Email EmailConf

func InitConfig() {
	Service = ServiceConf{
		Port:             getEnv("PORT", "8080"),
		BlogSyncInterval: getEnv("SUBSCRIPTION_SYNC_INTERVAL", "@every 30m"),
	}

	syncMaxSize, _ := strconv.Atoi(getEnv("SYNC_MAX_SIZE", ""))
	if syncMaxSize == 0 {
		syncMaxSize = 3
	}
	Notion = NotionConf{
		NotionApiKey: getEnv("NOTION_API_KEY", ""),
		NotionDBID:   getEnv("NOTION_DATABASE_ID", ""),
		SyncMaxSize:  syncMaxSize,
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
