package config

import (
	"os"
)

type ServiceConf struct {
	Port             string
	BlogSyncInterval string
}

type NotionConf struct {
	NotionApiKey   string
	NotionRssDBID  string
	NotionPostDBID string
}

type AIConf struct {
	KimiSecretKey string
	KimiModel     string
}

var Service ServiceConf
var Notion NotionConf
var AI AIConf

func InitConfig() {
	Service = ServiceConf{
		Port:             getEnv("PORT", "8080"),
		BlogSyncInterval: getEnv("SUBSCRIPTION_SYNC_INTERVAL", "@every 1h"),
	}

	Notion = NotionConf{
		NotionApiKey:   getEnv("NOTION_API_KEY", ""),
		NotionRssDBID:  getEnv("NOTION_RSS_DATABASE_ID", ""),
		NotionPostDBID: getEnv("NOTION_POST_DATABASE_ID", ""),
	}

	AI = AIConf{
		KimiSecretKey: getEnv("MOONSHOT_API_KEY", ""),
		KimiModel:     getEnv("KIMI_MODEL", "moonshot-v1-32k"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
