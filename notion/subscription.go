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

type Subscription struct {
	ID       string
	PostDbID string
	Name     string
	URL      string
	Posts    []*Post
}

type Post struct {
	ID          string
	Title       string
	CNTitle     string
	Authors     string
	Link        string
	PublishTime time.Time
	Content     string
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

	makeSummarize(subscriptions)

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

	return nil
}

func querySubscriptionsInNotion() ([]*Subscription, error) {
	dbItems, err := notionAPI.FetchDatabaseItems(config.Notion.NotionDBID,
		[]notionAPI.DatabaseFilter{
			{
				Property: "Category",
				Select:   map[string]string{"equals": "RSS"},
			},
			{
				Property: "Enabled",
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
		s := Subscription{
			ID:   item.ID,
			Name: prop["Name"].Title[0].PlainText,
			URL:  prop["URL"].URL,
		}

		blocks, err := notionAPI.FetchBlockChilds(item.ID)
		if err != nil {
			log.Printf("FetchBlockChilds error, ID:%s, Name:%s, err:%v\n", s.ID, s.Name, err)
			continue
		}
		if len(blocks) == 0 {
			continue
		}

		database := blocks[0]
		if database.Type != "child_database" {
			continue
		}

		s.PostDbID = database.ID

		log.Printf("%d. %s: %s\n", i+1, s.Name, s.URL)
		subscriptions = append(subscriptions, &s)
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
			filters := make([]notionAPI.DatabaseFilter, len(s.Posts))
			for i, post := range s.Posts {
				filters[i] = notionAPI.DatabaseFilter{
					Property: "Link",
					URL:      map[string]string{"equals": post.Link},
				}
			}
			existPosts, err := notionAPI.FetchDatabaseItems(s.PostDbID, filters, notionAPI.OR)
			if err != nil {
				log.Printf("query exist posts error:%v", err)
				s.Posts = nil
				return
			}

			if len(existPosts) == 0 {
				return
			}

			existPostsMap := map[string]struct{}{}
			for _, p := range existPosts {
				link := p.Properties["Link"].URL
				existPostsMap[link] = struct{}{}
			}

			var newPosts []*Post
			for _, p := range s.Posts {
				if _, exist := existPostsMap[p.Link]; exist {
					continue
				}
				newPosts = append(newPosts, p)
			}

			s.Posts = newPosts
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

				content := item.Content
				if content == "" {
					content = item.Description
				}
				post := Post{ID: item.GUID, Title: item.Title, Link: item.Link, Content: content}

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
					log.Printf("publish time %s parse error:%v\n", item.Published, err)
				}
				post.PublishTime = publishTime

				posts = append(posts, &post)
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

func makeSummarize(subscriptions []*Subscription) {
	var posts []*Post
	for _, s := range subscriptions {
		posts = append(posts, s.Posts...)
	}
	if len(posts) == 0 {
		log.Println("Not any posts.")
		return
	}

	log.Println("Begin to summarize posts...")
	for _, post := range posts {
		log.Printf("summarize post, %s: \"%s\" \n", post.Authors, post.Title)
		err := post.summarize()
		if err != nil {
			log.Printf("summarize post %s error:%v\n", post.Title, err)
			continue
		}
	}

	return
}

func (s *Subscription) savePostSummaryToNotion() error {
	if len(s.Posts) == 0 {
		log.Printf("[%s] not any new posts", s.Name)
		return nil
	}

	for _, post := range s.Posts {
		databaseID := s.PostDbID
		log.Printf("[%s] save summary to notion, title:%s\n", databaseID, post.Title)
		err := post.saveSummaryToNotion(databaseID)
		if err != nil {
			log.Printf("saveSummaryToNotion error, Name:%s, err:%v\n", post.Title, err)
			break
		}
	}
	return nil
}

func (post *Post) summarize() error {
	return retry.Do(
		func() error {
			prompt := post.Link
			if post.Content != "" {
				prompt = post.Content
			}

			plainSummary, err := kimi.SendChatRequest(prompt)
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
		"CN Title": {
			RichText: []notionAPI.RichTextProperty{
				{Text: notionAPI.TextField{Content: post.CNTitle}},
			},
		},
		"Published": {
			Date: &notionAPI.DateProperty{
				Start: post.PublishTime.Format("2006-01-02 15:04:05"),
			},
		},
		"Link": {URL: post.Link},
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
