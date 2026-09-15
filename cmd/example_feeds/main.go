package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"time"

	"github.com/meesooqa/go-cfg"
	"github.com/meesooqa/go-lgr"

	"github.com/feed-relay/contracts"

	"github.com/feed-relay/smotrim/internal/writer"
	"github.com/feed-relay/smotrim/pkg/feed_provider"
)

type AppConfig struct {
	Logger lgr.Config `yaml:"logger"`
	HTTP   struct {
		Timeout time.Duration `yaml:"timeout"`
	} `yaml:"http"`
	RawTestData         bool   `yaml:"test_data"`
	RawOutputDir        string `yaml:"output_dir"`
	RawItunesOwnerName  string `yaml:"itunes_owner_name"`
	RawItunesOwnerEmail string `yaml:"itunes_owner_email"`
	RawGenerator        string `yaml:"generator"`
	RawFeeds            []Feed `yaml:"feeds"`
}

type Feed struct {
	RawLimitPerShow int      `yaml:"limit_per_show"`
	RawSlug         string   `yaml:"slug"`
	RawShows        []string `yaml:"shows"`
	RawLink         string   `yaml:"link"`
	RawTitle        string   `yaml:"title"`
	RawDescription  string   `yaml:"description"`
	RawImage        string   `yaml:"image"`
}

func main() {
	configPath := flag.String("config", "etc/config.yml", "path to the YAML config file")
	prod := flag.Bool("prod", false, "Use production data instead of test data")
	outDir := flag.String("out", "", "xml feeds output dir")
	flag.Parse()

	conf, err := cfg.Load[AppConfig](*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if *prod {
		conf.RawTestData = false
	}
	logger, err := lgr.New(conf.Logger)
	if err != nil {
		log.Fatal(err)
	}
	slog.SetDefault(logger)

	p := feed_provider.NewProvider(conf)

	ctx := context.Background()
	feeds, err := p.Feeds(ctx, conf.Feeds())
	if err != nil {
		slog.Error("Run", slog.Any("err", err))
	}

	w := writer.NewXmlFileWriter()
	if *outDir != "" {
		conf.RawOutputDir = *outDir
	}
	if conf.OutputDir() == "" {
		slog.Error("OutputDir is empty")
		slog.Debug("feeds", slog.Any("feeds", feeds))
		return
	}
	for slug, feed := range feeds {
		err = w.Write(conf.OutputDir(), slug, feed)
		if err != nil {
			slog.Error("write", slog.Any("err", err))
		}
	}
}

func (c *AppConfig) Feeds() []contracts.Feed {
	feeds := make([]contracts.Feed, len(c.RawFeeds))
	for i, f := range c.RawFeeds {
		feeds[i] = f
	}
	return feeds
}

func (c *AppConfig) HTTPTimeout() time.Duration {
	return c.HTTP.Timeout
}

func (c *AppConfig) TestData() bool {
	return c.RawTestData
}

func (c *AppConfig) OutputDir() string {
	return c.RawOutputDir
}

func (c *AppConfig) ItunesOwnerName() string {
	return c.RawItunesOwnerName
}

func (c *AppConfig) ItunesOwnerEmail() string {
	return c.RawItunesOwnerEmail
}

func (c *AppConfig) Generator() string {
	return c.RawGenerator
}

func (f Feed) Limit() int {
	return f.RawLimitPerShow
}

func (f Feed) Slug() string {
	return f.RawSlug
}

func (f Feed) Shows() []string {
	return f.RawShows
}

func (f Feed) Link() string {
	return f.RawLink
}

func (f Feed) Title() string {
	return f.RawTitle
}

func (f Feed) Description() string {
	return f.RawDescription
}

func (f Feed) Image() string {
	return f.RawImage
}
