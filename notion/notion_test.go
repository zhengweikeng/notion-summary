package notion

import (
	"testing"
)

func TestQueryBlogs(t *testing.T) {
	blogs, err := QueryPosts()
	if err != nil {
		t.Errorf("should not got error, but got err:%v", err)
		return
	}

	for _, b := range blogs {
		t.Logf("Title:%s", b.Title)
		t.Logf("Published:%s", b.PublishTime)
		t.Logf("Author:%s", b.Author)
		t.Logf("Link:%s", b.Link)
		t.Logf("IsStar:%v", b.IsStar)
		t.Log("------------------")
	}
}
