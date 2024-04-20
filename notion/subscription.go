package notion

import (
	"fmt"
	"log"
	"notion-summary/config"
	"notion-summary/kimi"
	notionAPI "notion-summary/notion/api"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go"
	"github.com/mmcdole/gofeed"
)

const (
	TEMP_PROP_NAME        = "Name"         // 名字
	TEMP_PROP_URL         = "URL"          // RSS链接
	TEMP_PROP_LASTUPDATED = "Last Updated" // 最新一篇文章的发布时间
	TEMP_PROP_CATEGORY    = "Category"     // 数据类别
	TEMP_PROP_ENABLED     = "Enabled"      // 是否启用
)

type Subscription struct {
	ID              string
	Name            string
	URL             string
	Posts           []*Post
	LastUpdatedTime time.Time
	HasUpdated      bool
}

type Post struct {
	Title       string
	CNTitle     string
	Authors     string
	Link        string
	PublishTime time.Time
	Summary     string
}

func QuerySubscriptions() ([]*Subscription, error) {
	subscriptions, err := querySubscriptionsInNotion()
	if err != nil {
		log.Printf("querySubscriptionsInNotion error:%v\n", err)
		return nil, err
	}

	if len(subscriptions) == 0 {
		log.Println("Not any subscriptions")
		return nil, nil
	}

	log.Println("Begin to fetch posts according to your subscriptions...")
	err = fetchPosts(subscriptions)
	if err != nil {
		return nil, err
	}

	err = makeSummarize(subscriptions)
	if err != nil {
		return nil, err
	}

	return subscriptions, nil
}

// UpdateSubscriptionsInfos 将包含总结的信息写入notion中
func UpdateSubscriptionsInfos(subscriptions []*Subscription) error {
	var wg sync.WaitGroup
	wg.Add(len(subscriptions))

	for _, subscription := range subscriptions {
		go func(s *Subscription) {
			defer wg.Done()

			s.savePostSummaryToNotion()
		}(subscription)
	}

	wg.Wait()

	syncLastUpdatedTime(subscriptions)

	return nil
}

func querySubscriptionsInNotion() ([]*Subscription, error) {
	dbItems, err := notionAPI.FetchDatabaseItems(config.Notion.NotionDBID,
		[]notionAPI.DatabaseFilter{
			{
				Property: TEMP_PROP_CATEGORY,
				Select:   map[string]string{"equals": "RSS"},
			},
			{
				Property: TEMP_PROP_ENABLED,
				Checkbox: map[string]bool{"equals": true},
			},
		}, notionAPI.AND)
	if err != nil {
		log.Printf("fetch notion data error:%v\n", err)
		return nil, err
	}

	if len(dbItems) == 0 {
		return nil, nil
	}

	var subscriptions []*Subscription
	log.Println("Your subscription list:")
	for i, item := range dbItems {
		prop := item.Properties
		var lastUpdatedTime time.Time
		if prop[TEMP_PROP_LASTUPDATED].Date != nil {
			lastUpdatedTime, _ = parseDate(prop[TEMP_PROP_LASTUPDATED].Date.Start)
		}
		subscription := Subscription{
			ID:              item.ID,
			Name:            prop[TEMP_PROP_NAME].Title[0].PlainText,
			URL:             prop[TEMP_PROP_URL].URL,
			LastUpdatedTime: lastUpdatedTime,
		}

		log.Printf("%d. %s: %s\n", i+1, subscription.Name, subscription.URL)
		subscriptions = append(subscriptions, &subscription)
	}

	return subscriptions, nil
}

func fetchPosts(subscriptions []*Subscription) error {
	var wg sync.WaitGroup
	wg.Add(len(subscriptions))

	for _, subscription := range subscriptions {
		go func(s *Subscription) {
			defer wg.Done()
			s.fetchRSSPosts()
		}(subscription)
	}

	wg.Wait()
	return nil
}

func (s *Subscription) fetchRSSPosts() {
	err := retry.Do(
		func() error {
			fp := gofeed.NewParser()
			feed, err := fp.ParseURL(s.URL)
			if err != nil {
				return err
			}
			if feed == nil {
				return nil
			}

			var posts []*Post
			for _, item := range feed.Items {
				if config.Notion.SyncMaxSize > 0 && len(posts) >= config.Notion.SyncMaxSize {
					break
				}

				if item == nil {
					continue
				}

				post := Post{Title: item.Title, Link: item.Link}

				var authors []*gofeed.Person
				if len(item.Authors) > 0 {
					authors = item.Authors
				} else {
					authors = feed.Authors
				}
				authorNames := make([]string, len(authors))
				for i, author := range authors {
					authorNames[i] = author.Name
				}
				if len(authorNames) == 0 {
					authorNames = []string{s.Name}
				}
				post.Authors = strings.Join(authorNames, ",")

				publishTime, err := parseDate(item.Published)
				if err != nil {
					log.Printf("parse publish time error:%v\n", err)
					continue
				}
				post.PublishTime = publishTime

				if publishTime.After(s.LastUpdatedTime) {
					log.Printf("publishTime:%v lastUpdateTime:%v", publishTime, s.LastUpdatedTime)
					posts = append(posts, &post)
					continue
				}
			}

			s.Posts = posts
			return nil
		},
		retry.Attempts(5),
		retry.Delay(2*time.Second),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		log.Printf("fetch posts error, rss:%s, error:%v\n", s.URL, err)
		return
	}
}

func makeSummarize(subscriptions []*Subscription) error {
	var posts []*Post
	for _, s := range subscriptions {
		posts = append(posts, s.Posts...)
	}
	if len(posts) == 0 {
		log.Println("Not any posts.")
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(len(posts))

	log.Println("Begin to summarize posts...")
	for _, post := range posts {
		log.Printf("summarize post, %s: \"%s\" \n", post.Authors, post.Title)

		go func(post *Post) {
			defer wg.Done()
			err := post.summarize()
			if err != nil {
				log.Printf("summarize post %s error:%v\n", post.Title, err)
				return
			}
		}(post)
	}

	wg.Wait()

	return nil
}

func (s *Subscription) savePostSummaryToNotion() error {
	if len(s.Posts) == 0 {
		return nil
	}

	var blocks []notionAPI.Block
	blocks, err := notionAPI.FetchBlockChilds(s.ID)
	if err != nil {
		log.Printf("FetchBlockChilds error, ID:%s, Name:%s, err:%v\n", s.ID, s.Name, err)
		return err
	}
	if len(blocks) == 0 {
		return nil
	}

	database := blocks[0]
	if database.Type != "child_database" {
		return nil
	}

	for i, post := range s.Posts {
		log.Printf("[%s] save summary to notion, title:%s\n", database.ID, post.Title)
		err := post.saveSummaryToNotion(database.ID)
		if err != nil {
			log.Printf("saveSummaryToNotion error, Name:%s, err:%v\n", post.Title, err)
			break
		}

		if i == 0 {
			s.LastUpdatedTime = post.PublishTime
			s.HasUpdated = true
		}
	}
	return nil
}

func (post *Post) summarize() error {
	return retry.Do(
		func() error {
			plainSummary, err := kimi.SendChatRequest(post.Link)
			if err != nil {
				log.Printf("kimi error:%v\n", err)
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
}

func (post *Post) saveSummaryToNotion(databaseID string) error {
	pageProps := map[string]notionAPI.Property{
		"Name": {
			Title: []notionAPI.TitleProperty{
				{Text: notionAPI.TextField{Content: post.Title}},
			},
		},
		"Authors": {
			RichText: []notionAPI.RichTextProperty{
				{Text: notionAPI.TextField{Content: post.Authors}},
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
	children = append(children,
		notionAPI.Block{
			Object: "block",
			Type:   "paragraph",
			Paragraph: &notionAPI.BlockParagraph{
				RichText: []notionAPI.RichTextProperty{
					{Text: notionAPI.TextField{Content: fmt.Sprintf("作者：%s", post.Authors)}},
				},
			},
		},
		notionAPI.Block{
			Object: "block",
			Type:   "paragraph",
			Paragraph: &notionAPI.BlockParagraph{
				RichText: []notionAPI.RichTextProperty{
					{Text: notionAPI.TextField{Content: post.Summary}},
				},
			},
		})

	_, err := notionAPI.CreatePageInDatabase(databaseID, pageProps, children)
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
	summaryPrefix := "内容总结：\n"
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
			t = t.Truncate(time.Minute)
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}

func syncLastUpdatedTime(subscriptions []*Subscription) {
	var needUpdates []*Subscription
	for _, s := range subscriptions {
		if !s.HasUpdated {
			continue
		}
		needUpdates = append(needUpdates, s)
	}
	if len(needUpdates) == 0 {
		log.Println("Not any blogs need to update")
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(needUpdates))
	for _, subscription := range needUpdates {
		log.Printf("[%s] sync lastUpdatedTime:%v\n", subscription.Name, subscription.LastUpdatedTime)
		go func(s *Subscription) {
			defer wg.Done()

			lastUpdatedTime := s.LastUpdatedTime.Format("2006-01-02 15:04:05")
			pageProps := map[string]notionAPI.Property{
				"Last Updated": {
					Date: &notionAPI.DateProperty{Start: lastUpdatedTime},
				},
			}

			err := notionAPI.UpdatePage(s.ID, pageProps)
			if err != nil {
				log.Printf("UpdatePage error, Name:%s, err:%v\n", s.Name, err)
				return
			}
		}(subscription)
	}
}
