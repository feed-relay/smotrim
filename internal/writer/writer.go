package writer

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/feed-relay/rsscast"
)

type XmlFileWriter struct{}

func NewXmlFileWriter() *XmlFileWriter {
	return &XmlFileWriter{}
}

func (w *XmlFileWriter) Write(dir, slug string, feed *rsscast.Feed) error {
	path := filepath.Join(dir, slug+".xml")

	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			slog.Error("failed to close file", slog.Any("error", closeErr))
		}
	}()

	err = feed.Encode(f)
	if err != nil {
		return fmt.Errorf("feed encoding failed: %w", err)
	}

	return nil
}
