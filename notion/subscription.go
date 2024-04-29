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
	"github.com/russross/blackfriday/v2"
)

type Subscription struct {
	ID    string
	Name  string
	URL   string
	Posts []*Post
}

type Post struct {
	ID          string
	Title       string
	Authors     string
	Link        string
	PublishTime time.Time
	Content     string
	Summary     []notionAPI.Block
}

type Summary struct {
	Title   string
	Outline string
	Content string
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
	log.Printf("query db:%s", config.Notion.NotionRssDBID)
	dbItems, err := notionAPI.FetchDatabaseItems(config.Notion.NotionRssDBID,
		[]notionAPI.DatabaseFilter{
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
			existPosts, err := notionAPI.FetchDatabaseItems(config.Notion.NotionPostDBID, filters, notionAPI.OR)
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
				if len(posts) >= 1 {
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
}

func (s *Subscription) savePostSummaryToNotion() error {
	if len(s.Posts) == 0 {
		log.Printf("[%s] not any new posts", s.Name)
		return nil
	}

	postDBID := config.Notion.NotionPostDBID
	for _, post := range s.Posts {
		log.Printf("[%s] save summary to notion, title:%s\n", postDBID, post.Title)
		err := post.saveSummaryToNotion(postDBID)
		if err != nil {
			log.Printf("saveSummaryToNotion error, Name:%s, err:%v\n", post.Title, err)
			continue
		}
	}
	return nil
}

func (post *Post) summarize() error {
	return retry.Do(
		func() error {
			prompt := post.Link

			plainSummary, err := kimi.SendChatRequest(prompt)
			if err != nil {
				log.Printf("kimi error:%v\n", err)
				return err
			}

			summary := parseSummary(plainSummary)
			post.Summary = summary
			return nil
		},
		retry.Attempts(5),
		retry.Delay(2*time.Second),
		retry.DelayType(retry.BackOffDelay),
	)
}

func (post *Post) saveSummaryToNotion(databaseID string) error {
	summary := post.Summary
	if summary == nil {
		return nil
	}

	cnTitle := ""
	outline := ""

	var prevBlock notionAPI.Block
	for _, block := range post.Summary {
		if block.Heading2 != nil {
			cnTitle = block.Heading2.RichText[0].Text.Content
		} else if block.Paragraph != nil {
			content := block.Paragraph.RichText[0].Text.Content
			if prevBlock.Object == "" {
				if content == "概要" {
					prevBlock = block
				}
			} else {
				if content == "总结" {
					break
				}
				if prevBlock.Paragraph.RichText[0].Text.Content == "概要" {
					outline += content
				}
			}
		}
	}

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
				{Text: notionAPI.TextField{Content: cnTitle}},
			},
		},
		"Published": {
			Date: &notionAPI.DateProperty{
				Start: post.PublishTime.Format("2006-01-02 15:04:05"),
			},
		},
		"Link": {URL: post.Link},
		"Outline": {
			RichText: []notionAPI.RichTextProperty{
				{Text: notionAPI.TextField{Content: outline}},
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
	children = append(children, summary...)

	_, err := notionAPI.CreatePageInDatabase(databaseID, pageProps, children)
	return err
}

func parseSummary(plainSummary string) []notionAPI.Block {
	var blocks []notionAPI.Block

	node := blackfriday.New().Parse([]byte(plainSummary))
	node.Walk(func(node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		if entering {
			text := ""
			switch node.Type {
			case blackfriday.Heading:
				text = string(node.FirstChild.Literal)
				level := node.HeadingData.Level

				if level == 3 {
					blocks = append(blocks, notionAPI.Block{
						Object: "block",
						Type:   "paragraph",
						Paragraph: &notionAPI.BlockParagraph{
							RichText: []notionAPI.RichTextProperty{
								{
									Text: notionAPI.TextField{Content: text},
									Annotations: notionAPI.Annotations{
										Bold:  true,
										Color: "default",
									},
								},
							},
						},
					})
				}
			case blackfriday.Paragraph:
				text = string(node.FirstChild.Literal)
				blocks = append(blocks, notionAPI.Block{
					Object: "block",
					Type:   "paragraph",
					Paragraph: &notionAPI.BlockParagraph{
						RichText: []notionAPI.RichTextProperty{
							{Text: notionAPI.TextField{Content: text}},
						},
					},
				})
			}
		}

		return blackfriday.GoToNext
	})

	if len(blocks) == 0 {
		return nil
	}

	titleBlock := blocks[0]
	blocks[0] = notionAPI.Block{
		Object: "block",
		Type:   "heading_2",
		Heading2: &notionAPI.BlockHeading2{
			RichText: []notionAPI.RichTextProperty{
				{Text: notionAPI.TextField{Content: titleBlock.Paragraph.RichText[0].Text.Content}},
			},
		},
	}

	return blocks
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
