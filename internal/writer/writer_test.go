package writer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/feed-relay/rsscast"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXmlFileWriter_Write(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewXmlFileWriter()
	feed := rsscast.NewFeed(rsscast.FeedData{
		Title:       "Test Feed",
		Description: "Test Feed Description",
		Image:       "http://example.com/image.jpg",
		Language:    "en",
		Explicit:    rsscast.ExplicitFalse,
		Categories:  []rsscast.Category{{Text: "Test Category"}},
	})

	item := rsscast.NewItem(rsscast.ItemData{
		Title: "Test Item",
		Guid:  "http://example.com/item",
		Enclosure: rsscast.Enclosure{
			URL:    "http://example.com/audio.mp3",
			Type:   rsscast.Mp3,
			Length: 1000,
		},
	})
	feed.AddItem(item)

	t.Run("success", func(t *testing.T) {
		slug := "test-feed"
		err := w.Write(tmpDir, slug, feed)
		require.NoError(t, err)

		path := filepath.Join(tmpDir, slug+".xml")
		_, err = os.Stat(path)
		assert.NoError(t, err)
	})

	t.Run("nested directory", func(t *testing.T) {
		slug := "nested/test-feed"
		err := w.Write(tmpDir, slug, feed)
		require.NoError(t, err)

		path := filepath.Join(tmpDir, slug+".xml")
		_, err = os.Stat(path)
		assert.NoError(t, err)
	})

	t.Run("invalid directory", func(t *testing.T) {
		// Create a file where the directory should be to cause MkdirAll failure
		blockedDir := filepath.Join(tmpDir, "blocked")
		err := os.WriteFile(blockedDir, []byte("not a dir"), 0o600)
		require.NoError(t, err)

		slug := "blocked/test-feed"
		err = w.Write(tmpDir, slug, feed)
		assert.Error(t, err)
	})
}
