package api

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

const audioEndpoint = "audio"

type Audio struct {
	Id        int    `json:"id"`
	PublicId  int    `json:"publicId"`
	Duration  int64  `json:"duration"`
	ShareLink string `json:"shareLink"`
	Streams   struct {
		Mp3 string `json:"mp3"`
	} `json:"streams"`
}

type audioResult struct {
	Data *Audio `json:"data,omitempty"`
}

func (c *apiClient) Audio(ctx context.Context, publicId int) (*Audio, error) {
	fullURL, err := c.audioURL(publicId)
	if err != nil {
		return nil, fmt.Errorf("build url: %w", err)
	}

	var result audioResult
	err = c.get(ctx, fullURL, &result)
	if err != nil {
		return nil, fmt.Errorf("http get %s: %w", fullURL, err)
	}

	if result.Data == nil {
		return nil, errors.New("response contains no audio data")
	}

	return result.Data, nil
}

func (c *apiClient) audioURL(publicId int) (string, error) {
	base, err := url.Parse(c.baseURL())
	if err != nil {
		return "", fmt.Errorf("parse base url %q: %w", c.baseURL(), err)
	}
	return base.JoinPath(audioEndpoint, strconv.Itoa(publicId)).String(), nil
}
