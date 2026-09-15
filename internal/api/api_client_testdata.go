package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
)

//go:embed testdata/*
var testData embed.FS

type testdataApiClient struct{}

func (c *testdataApiClient) Audio(_ context.Context, _ int) (*Audio, error) {
	var res audioResult
	err := c.do("testdata/audio.json", &res)
	return res.Data, err
}

func (c *testdataApiClient) do(filename string, result any) error {
	file, err := testData.Open(filename)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("failed to close file", slog.Any("error", err))
		}
	}()

	if result != nil {
		if err := json.NewDecoder(file).Decode(result); err != nil {
			return fmt.Errorf("decode file: %w", err)
		}
	}

	return nil
}
