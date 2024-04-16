package config

import (
	"os"
	"strconv"
)

type NotionConf struct {
	NotionApiKey  string
	NotionDBID    string
	NotionVersion string
}

type AIConf struct {
	ConcurrentNum int
	RPM           int // request per minute
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

	concurrentNum, _ := strconv.Atoi(getEnv("CONCURRENT_NUM", "1"))
	if concurrentNum == 0 {
		concurrentNum = 1
	}
	rpm, _ := strconv.Atoi(getEnv("RPM", "3"))
	if rpm == 0 {
		rpm = 3
	}

	AI = AIConf{
		ConcurrentNum: concurrentNum,
		RPM:           rpm,
		KimiSecretKey: getEnv("MOONSHOT_API_KEY", ""),
		KimiModel:     getEnv("KIMI_MODEL", "moonshot-v1-8k"),
	}

	Email = EmailConf{
		APIKey: getEnv("RESEND_API_KEY", ""),
		FROM:   getEnv("RESEND_FROM", "onboarding@resend.dev"),
		To:     getEnv("RESEND_To", "seed1029zwk@gmail.com"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
