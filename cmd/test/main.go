package main

import (
	"flag"
	"log"
	"log/slog"
	"time"

	"github.com/meesooqa/go-cfg"
	"github.com/meesooqa/go-lgr"
)

type AppConfig struct {
	Logger lgr.Config `yaml:"logger"`
	HTTP   struct {
		Timeout time.Duration `yaml:"timeout"`
	} `yaml:"http"`
	RawTestData bool `yaml:"test_data"`
	Feeds       Feed `yaml:"feeds"`
}

type Feed struct {
	RawLimitPerShow int      `yaml:"limit_per_show"`
	RawSlug         string   `yaml:"slug"`
	RawShows        []string `yaml:"shows"`
}

func main() {
	configPath := flag.String("config", "etc/config.yml", "path to the YAML config file")
	prod := flag.Bool("prod", false, "Use production data instead of test data")
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
}

func (f Feed) LimitPerShow() int {
	return f.RawLimitPerShow
}

func (f Feed) Slug() string {
	return f.RawSlug
}

func (f Feed) Shows() []string {
	return f.RawShows
}
