package main

import (
	"log"
	"net/http"
	"notion-summary/config"
	"notion-summary/notion"
)

func main() {
	log.Println("Initialize config")
	config.InitConfig()

	log.Println("Initialize cron jobs")
	notion.InitCronJobs()

	http.ListenAndServe(":"+config.Service.Port, nil)
}
