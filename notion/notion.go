package notion

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"strings"
	"summary-notion/config"
	"summary-notion/kimi"
	"summary-notion/notification"
	notionAPI "summary-notion/notion/api"
	"sync"
	"time"

	"github.com/avast/retry-go"
	"github.com/mmcdole/gofeed"
	"github.com/robfig/cron/v3"
)

const (
	TEMP_PROP_NAME        = "Name" // 博客名字
	TEMP_PROP_URL         = "URL"  // RSS链接
	TEMP_PROP_LASTUPDATED = "Last Updated"
	TEMP_PROP_CATEGORY    = "Category" // 数据类别，Blog
)

type Blog struct {
	ID              string
	Name            string
	URL             string
	Posts           []*BlogPost
	LastUpdatedTime time.Time
	HasUpdated      bool
}

type BlogPost struct {
	BlogName    string
	Title       string
	CNTitle     string
	Author      string
	Link        string
	PublishTime time.Time
	Summary     string
}

type NoticeTemplate struct {
	Posts []BlogPost
}

func StartJobs() {
	c := cron.New(cron.WithSeconds())

	startSummaryBlogJob(c)

	c.Start()
}

func startSummaryBlogJob(c *cron.Cron) {
	var summaryJob = func() {
		log.Println("start QueryPosts...")
		blogs, err := QueryBlogs()
		if err != nil {
			log.Printf("QueryPosts error:%v", err)
			return
		}
		if len(blogs) == 0 {
			log.Println("Not any blogs")
			return
		}

		log.Println("start SummarizePosts...")
		err = SummarizePosts(blogs)
		if err != nil {
			log.Printf("SummarizePosts error:%v", err)
			return
		}

		log.Println("start SaveBlogSummariesToNotion...")
		err = SaveBlogSummariesToNotion(blogs)
		if err != nil {
			log.Printf("SaveBlogSummariesToNotion error:%v", err)
			return
		}

		err = UpdateBlogUpdateTimeToNotion(blogs)
		if err != nil {
			log.Printf("UpdateBlogUpdateTimeToNotion error:%v", err)
			return
		}
	}
	summaryJob()

	c.AddFunc(config.Service.BlogSyncInterval, summaryJob)
}

// QueryPosts 查询博客
// 从notion中的database中查询上一天到当前发布的所有博客
func QueryBlogs() ([]*Blog, error) {
	blogs, err := getBlogs()
	if err != nil {
		log.Printf("getBlogsFromNotion error:%v", err)
		return nil, err
	}

	if len(blogs) == 0 {
		return nil, nil
	}

	var wg sync.WaitGroup
	wg.Add(len(blogs))

	for _, blog := range blogs {
		go func(b *Blog) {
			defer wg.Done()

			err := retry.Do(
				func() error { return fetchPosts(b) },
				retry.Attempts(5),
				retry.Delay(2*time.Second),
				retry.DelayType(retry.BackOffDelay),
			)
			if err != nil {
				log.Printf("fetch blog error, rss:%s, error:%v", b.URL, err)
				return
			}
		}(blog)
	}

	wg.Wait()

	return blogs, nil
}

func getBlogs() ([]*Blog, error) {
	dbItems, err := notionAPI.FetchDatabaseItems(config.Notion.NotionDBID, &notionAPI.DatabaseFilter{
		Property: TEMP_PROP_CATEGORY,
		Select:   map[string]interface{}{"equals": "Blog"},
	})
	if err != nil {
		log.Printf("fetch notion data error:%v", err)
		return nil, err
	}

	var blogs []*Blog
	for _, item := range dbItems {
		prop := item.Properties
		var lastUpdatedTime time.Time
		if prop[TEMP_PROP_LASTUPDATED].Date != nil {
			lastUpdatedTime, _ = parseDate(prop[TEMP_PROP_LASTUPDATED].Date.Start)
		}
		blog := Blog{
			ID:              item.ID,
			Name:            prop[TEMP_PROP_NAME].Title[0].PlainText,
			URL:             prop[TEMP_PROP_URL].URL,
			LastUpdatedTime: lastUpdatedTime,
		}

		blogs = append(blogs, &blog)
	}

	return blogs, nil
}

// SummarizePosts
// 调用AI总结每篇博客的内容
func SummarizePosts(blogs []*Blog) error {
	var posts []*BlogPost
	for _, b := range blogs {
		posts = append(posts, b.Posts...)
	}
	if len(posts) == 0 {
		log.Println("Not any posts.")
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(len(posts))

	for i, post := range posts {
		go func(i int, post *BlogPost) {
			defer wg.Done()
			summarizePost(post)
		}(i, post)
	}

	wg.Wait()

	return nil
}

func summarizePost(post *BlogPost) error {
	err := retry.Do(
		func() error {
			plainSummary, err := kimi.SendChatRequest(post.Link)
			if err != nil {
				log.Printf("kimi error:%v", err)
				return err
			}

			cnTitle, summary := parseSummary(plainSummary)

			post.CNTitle = cnTitle
			post.Summary = summary
			return nil
		},
		retry.Attempts(5),
		retry.Delay(2*time.Second),
		retry.DelayType(retry.BackOffDelay),
	)

	return err
}

func parseSummary(plainSummary string) (title string, summary string) {
	// 提取标题
	titlePrefix := "标题："
	titleStart := strings.Index(plainSummary, titlePrefix)
	if titleStart == -1 {
		summary = plainSummary
		return
	}
	titleStart += len(titlePrefix)
	titleEnd := strings.Index(plainSummary[titleStart:], "\n")
	title = plainSummary[titleStart : titleStart+titleEnd]

	// 提取内容总结
	summaryPrefix := "内容总结："
	summaryStart := strings.Index(plainSummary, summaryPrefix)
	if summaryStart == -1 {
		summary = plainSummary
		return
	}
	summaryStart += len(summaryPrefix)
	summaryEnd := strings.Index(plainSummary[summaryStart:], "\n\n") // 假设内容总结后有两个换行表示段落结束
	if summaryEnd == -1 {
		summaryEnd = len(plainSummary) - summaryStart
	}
	summary = plainSummary[summaryStart : summaryStart+summaryEnd]

	return
}

// SaveBlogSummariesToNotion 将包含总结的博客信息写入notion中
func SaveBlogSummariesToNotion(blogs []*Blog) error {
	var wg sync.WaitGroup
	wg.Add(len(blogs))

	for _, blog := range blogs {
		go func(b *Blog) {
			defer wg.Done()

			if len(b.Posts) == 0 {
				return
			}

			var blocks []notionAPI.Block
			blocks, err := notionAPI.FetchBlockChilds(b.ID)
			if err != nil {
				log.Printf("FetchBlockChilds error, ID:%s, Name:%s, err:%v", b.ID, b.Name, err)
				return
			}
			if len(blocks) == 0 {
				return
			}

			database := blocks[0]
			if database.Type != "child_database" {
				return
			}

			for i, post := range b.Posts {
				pageProps := map[string]notionAPI.Property{
					"Name": {
						Title: []notionAPI.TitleProperty{
							{Text: notionAPI.TextField{Content: post.Title}},
						},
					},
					"CN Name": {
						RichText: []notionAPI.RichTextProperty{
							{Text: notionAPI.TextField{Content: post.CNTitle}},
						},
					},
					"Published": {
						Date: &notionAPI.DateProperty{
							Start: post.PublishTime.Format("2006-01-02 15:04:05"),
						},
					},
				}

				children := []notionAPI.Block{
					{
						Object:   "block",
						Type:     "bookmark",
						Bookmark: &notionAPI.BlockBookmark{URL: post.Link},
					},
				}
				if post.CNTitle != "" {
					children = append(children, notionAPI.Block{
						Object: "block",
						Type:   "heading_2",
						Heading2: &notionAPI.BlockHeading2{
							RichText: []notionAPI.RichTextProperty{
								{Text: notionAPI.TextField{Content: post.CNTitle}},
							},
						},
					})
				}
				children = append(children, notionAPI.Block{
					Object: "block",
					Type:   "paragraph",
					Paragraph: &notionAPI.BlockParagraph{
						RichText: []notionAPI.RichTextProperty{
							{Text: notionAPI.TextField{Content: post.Summary}},
						},
					},
				})

				_, err = notionAPI.CreatePageInDatabase(database.ID, pageProps, children)
				if err != nil {
					log.Printf("CreatePageInDatabase error, Name:%s, err:%v", post.Title, err)
					break
				}

				if i == 0 {
					b.LastUpdatedTime = post.PublishTime
					b.HasUpdated = true
				}
			}
		}(blog)
	}

	wg.Wait()

	return nil
}

func UpdateBlogUpdateTimeToNotion(blogs []*Blog) error {
	var needUpdateBlogs []*Blog
	for _, b := range blogs {
		if !b.HasUpdated {
			continue
		}
		needUpdateBlogs = append(needUpdateBlogs, b)
	}
	if len(needUpdateBlogs) == 0 {
		log.Println("Not any blogs need to update")
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(len(needUpdateBlogs))
	for _, blog := range needUpdateBlogs {
		go func(b *Blog) {
			defer wg.Done()

			lastUpdatedTime := b.LastUpdatedTime.Format("2006-01-02 15:04:05")
			pageProps := map[string]notionAPI.Property{
				"Last Updated": notionAPI.Property{
					Date: &notionAPI.DateProperty{Start: lastUpdatedTime},
				},
			}

			err := notionAPI.UpdatePage(b.ID, pageProps)
			if err != nil {
				log.Printf("UpdatePage error, Name:%s, err:%v", b.Name, err)
				return
			}
		}(blog)
	}

	return nil
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

func fetchPosts(b *Blog) (err error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(b.URL)
	if err != nil {
		return
	}
	if feed == nil {
		return
	}

	var posts []*BlogPost
	for _, item := range feed.Items {
		if config.Notion.SyncMaxSize > 0 && len(posts) >= config.Notion.SyncMaxSize {
			break
		}

		if item == nil {
			continue
		}

		post := BlogPost{BlogName: b.Name, Title: item.Title, Link: item.Link}

		if item.Author != nil {
			post.Author = item.Author.Name
		}

		publishTime, err := parseDate(item.Published)
		if err != nil {
			log.Printf("parse publish time error:%v", err)
			continue
		}
		post.PublishTime = publishTime

		if publishTime.After(b.LastUpdatedTime) {
			posts = append(posts, &post)
			continue
		}
	}

	b.Posts = posts

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
