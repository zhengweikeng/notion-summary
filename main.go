package main

import (
	"log"
	"net/http"
	"summary-notion/config"
	"summary-notion/notion"
)

func main() {
	log.Println("Initialize config")
	config.InitConfig()

	log.Println("Initialize cron jobs")
	notion.InitCronJobs()

	http.ListenAndServe(":"+config.Service.Port, nil)
}
