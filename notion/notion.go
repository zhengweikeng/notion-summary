package notion

import (
	"log"
	"notion-summary/config"

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

	_, err := c.AddFunc(config.Service.BlogSyncInterval, summaryJob)
	if err != nil {
		log.Printf("err:%v", err)
		return
	}
}
