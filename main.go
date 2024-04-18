package main

import (
	"log"
	"net/http"
	"summary-notion/config"
	"summary-notion/notion"
)

func main() {
	log.Println("Start init config")
	config.InitConfig()

	notion.StartJobs()
	log.Println("Start Jobs")

	http.ListenAndServe(":"+config.Service.Port, nil)
}
