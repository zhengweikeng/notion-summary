package notion

import (
	"bytes"
	"fmt"
	"focus-ai/config"
	"focus-ai/kimi"
	"focus-ai/notification"
	"html/template"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/avast/retry-go"
	"github.com/juju/ratelimit"
	"github.com/mmcdole/gofeed"
	"github.com/robfig/cron/v3"
)

// 获取notion database中的所有订阅链接

type BlogPost struct {
	Title       string
	Author      string
	Link        string
	BlogAddr    string
	PublishTime time.Time
	Summary     template.HTML
}

type NoticeTemplate struct {
	Posts []BlogPost
}

var bucket *ratelimit.Bucket
var semaphore chan struct{}
var once sync.Once

func StartJobs() {
	c := cron.New(cron.WithSeconds())

	startBlogUpdateNotificationJob(c)

	c.Start()
}

func startBlogUpdateNotificationJob(c *cron.Cron) {
	c.AddFunc("0 0 6 * * *", func() {
		log.Println("start QueryPosts...")
		posts, err := QueryPosts()
		if err != nil {
			log.Printf("QueryPosts error:%v", err)
			return
		}

		log.Println("start SummarizePosts...")
		err = SummarizePosts(posts)
		if err != nil {
			log.Printf("SummarizePosts error:%v", err)
			return
		}

		log.Println("start SendBlogsNotification...")
		err = SendBlogsNotification(posts)
		if err != nil {
			log.Printf("SendBlogsNotification error:%v", err)
			return
		}
	})
}

// QueryPosts 查询博客
// 从notion中的database中查询上一天到当前发布的所有博客
func QueryPosts() ([]BlogPost, error) {
	dbItems, err := fetchDatabase(BLOG)
	if err != nil {
		log.Printf("fetch notion data error:%v", err)
		return nil, err
	}

	var posts []BlogPost
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(dbItems))
	previousDay := time.Now().Local().AddDate(0, 0, -1)
	startOfPreviousDay := time.Date(previousDay.Year(),
		previousDay.Month(),
		previousDay.Day(),
		0, 0, 0, 0,
		previousDay.Location())

	for _, dbItem := range dbItems {
		prop := dbItem.Properties
		go func(rssURL string, isStar bool) {
			defer wg.Done()

			err := retry.Do(func() error {
				post, err := getLatestBlogFromRSS(rssURL, startOfPreviousDay)
				if err != nil {
					log.Printf("fetch blog error, rss:%s, error:%v", rssURL, err)
					return err
				}

				if post.Title == "" {
					return nil
				}

				mu.Lock()
				defer mu.Unlock()
				posts = append(posts, post)
				return nil
			})
			if err != nil {
				log.Printf("retry fetch blog error, rss:%s, error:%v", rssURL, err)
				return
			}

		}(prop[TEMP_PROP_RSS].URL, prop[TEMP_PROP_IS_STSR].Checkbox)
	}

	wg.Wait()

	sort.Slice(posts, func(i, j int) bool {
		return posts[i].PublishTime.After(posts[j].PublishTime)
	})

	return posts, nil
}

// SummarizePosts
// 调用AI总结每篇博客的内容
func SummarizePosts(posts []BlogPost) error {
	once.Do(func() {
		bucket = ratelimit.NewBucket(time.Minute, int64(config.AI.RPM))
		semaphore = make(chan struct{}, config.AI.ConcurrentNum)
	})

	var wg sync.WaitGroup
	wg.Add(len(posts))

	for i, post := range posts {
		go func(i int, post BlogPost) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			bucket.Wait(1)
			summary, err := summarizePost(post)
			if err != nil {
				log.Printf("summarizing post error: %v", err)
				summary = "Error obtaining summary"
			}

			posts[i].Summary = template.HTML(summary)
		}(i, post)
	}

	wg.Wait()

	return nil
}

func summarizePost(post BlogPost) (string, error) {
	var summary string
	err := retry.Do(
		func() error {
			tempSummary, err := kimi.SendChatRequest(post.Link)
			if err != nil {
				log.Printf("kimi error:%v", err)
				return err
			}

			summary = tempSummary
			return nil
		},
		retry.Attempts(3),
		retry.Delay(time.Second),
	)

	return summary, err
}

// SendBlogsNotification
// 发送博客总结通知，如邮件
func SendBlogsNotification(posts []BlogPost) error {
	subject := "您关注的博客有更新啦！"
	to := config.Email.To

	tmpl, err := template.ParseFiles("template/blog.html")
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, NoticeTemplate{Posts: posts})
	if err != nil {
		log.Printf("render notice template error:%v", err)
		return err
	}
	content := buf.String()

	return retry.Do(
		func() error { return notification.SendEmail(subject, to, content) },
		retry.Attempts(3),
		retry.Delay(time.Second),
	)
}

func getLatestBlogFromRSS(url string, startTime time.Time) (blog BlogPost, err error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(url)
	if err != nil {
		return
	}

	if feed == nil {
		return
	}

	blogAddr := feed.Link

	for _, item := range feed.Items {
		if item == nil {
			continue
		}

		blog := BlogPost{
			Title:    item.Title,
			Link:     item.Link,
			BlogAddr: blogAddr,
		}

		if item.Author != nil {
			blog.Author = item.Author.Name
		}

		if item.Published == "" {
			continue
		}

		publishTime, err := parseDate(item.Published)
		if err != nil {
			log.Printf("parse publish time error:%v", err)
			continue
		}
		if publishTime.Before(startTime) {
			continue
		}

		blog.PublishTime = publishTime

		return blog, nil
	}

	return
}

// parseDate tries to parse a date string into a time.Time object using a list of common RSS date formats.
func parseDate(dateStr string) (time.Time, error) {
	var layouts = []string{
		time.RFC1123,                   // Mon, 02 Jan 2006 15:04:05 MST
		time.RFC1123Z,                  // Mon, 02 Jan 2006 15:04:05 -0700
		time.RFC822,                    // 02 Jan 06 15:04 MST
		time.RFC822Z,                   // 02 Jan 06 15:04 -0700
		"Mon, 2 Jan 2006 15:04:05 MST", // Some feeds use a variant of RFC1123 with no leading zero on the day
		"2006-01-02T15:04:05Z07:00",    // ISO 8601 with timezone offset
		"2006-01-02T15:04:05Z",         // ISO 8601 UTC
	}

	for _, layout := range layouts {
		t, err := time.Parse(layout, dateStr)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}
