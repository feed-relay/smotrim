package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"time"

	"github.com/meesooqa/go-cfg"
	"github.com/meesooqa/go-lgr"

	"github.com/feed-relay/smotrim/internal/service"
)

type AppConfig struct {
	Logger lgr.Config `yaml:"logger"`
	HTTP   struct {
		Timeout time.Duration `yaml:"timeout"`
	} `yaml:"http"`
	RawTestData bool `yaml:"test_data"`
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

	ctx := context.Background()

	updater := service.NewSubsUpdater(conf)
	err = updater.UpdateSubs(ctx)
	if err != nil {
		slog.Error("Run", slog.Any("error", err))
	}
}

func (c *AppConfig) HTTPTimeout() time.Duration {
	return c.HTTP.Timeout
}

func (c *AppConfig) TestData() bool {
	return c.RawTestData
}
