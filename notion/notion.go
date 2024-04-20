package notion

import (
	"log"
	"summary-notion/config"

	"github.com/robfig/cron/v3"
)

func InitCronJobs() {
	c := cron.New(cron.WithSeconds())

	DoSummaryJob(c)

	c.Start()
}

func DoSummaryJob(c *cron.Cron) {
	var summaryJob = func() {
		log.Println("QueryPosts...")
		subscriptions, err := QuerySubscriptions()
		if err != nil {
			log.Printf("QueryPosts error:%v\n", err)
			return
		}
		if len(subscriptions) == 0 {
			return
		}

		log.Println("UpdateSubscriptionsInfos...")
		err = UpdateSubscriptionsInfos(subscriptions)
		if err != nil {
			log.Printf("SaveBlogSummariesToNotion error:%v\n", err)
			return
		}
	}
	summaryJob()

	c.AddFunc(config.Service.BlogSyncInterval, summaryJob)
}
