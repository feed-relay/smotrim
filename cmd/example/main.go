package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"time"

	"github.com/feed-relay/contracts"
	"github.com/meesooqa/go-cfg"
	"github.com/meesooqa/go-lgr"

	"github.com/feed-relay/smotrim/internal/writer"
	"github.com/feed-relay/smotrim/pkg/provider"
)

type AppConfig struct {
	Logger              lgr.Config `yaml:"logger"`
	HTTP                httpConfig `yaml:"http"`
	RawTestData         bool       `yaml:"test_data"`
	RawOutputDir        string     `yaml:"output_dir"`
	RawItunesOwnerName  string     `yaml:"itunes_owner_name"`
	RawItunesOwnerEmail string     `yaml:"itunes_owner_email"`
	RawGenerator        string     `yaml:"generator"`
	Subs                []Sub      `yaml:"subscriptions"`
}

type httpConfig struct {
	Timeout time.Duration `yaml:"timeout"`
}

type Sub struct {
	RawLimit int      `yaml:"limit"`
	RawShows []string `yaml:"shows"`
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
		// flag > conf
		conf.RawTestData = false
	}
	logger, err := lgr.New(conf.Logger)
	if err != nil {
		log.Fatal(err)
	}
	slog.SetDefault(logger)

	p := provider.NewProvider(conf)

	ctx := context.Background()
	feeds, err := p.Feeds(ctx, conf.Subscriptions())
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

func (c *AppConfig) HTTPTimeout() time.Duration {
	return c.HTTP.Timeout
}

func (c *AppConfig) Subscriptions() []contracts.Subscription {
	subscriptions := make([]contracts.Subscription, len(c.Subs))
	for i, sub := range c.Subs {
		subscriptions[i] = sub
	}
	return subscriptions
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

func (s Sub) Limit() int {
	return s.RawLimit
}

func (s Sub) Shows() []string {
	return s.RawShows
}
