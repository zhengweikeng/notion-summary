package main

import (
	"focus-ai/config"
	"focus-ai/notion"
	"log"
)

func main() {
	log.Println("Start init config")
	config.InitConfig()
	notion.StartJobs()

	log.Println("Start Jobs")
	select {}
}
