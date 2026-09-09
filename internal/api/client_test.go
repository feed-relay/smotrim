package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	t.Run("with timeout", func(t *testing.T) {
		timeout := 10 * time.Second
		client := NewClient(timeout)
		assert.NotNil(t, client)
	})

	t.Run("without timeout", func(t *testing.T) {
		client := NewClient(0)
		assert.NotNil(t, client)
	})
}

func TestNewTestdataClient(t *testing.T) {
	client := NewTestdataClient()
	assert.NotNil(t, client)
}

func TestClient_BrandEpisodes(t *testing.T) {
	client := NewTestdataClient()
	ctx := context.Background()
	_, err := client.BrandEpisodes(ctx, 1, 10)
	assert.NoError(t, err)
}

func TestClient_Audio(t *testing.T) {
	client := NewTestdataClient()
	ctx := context.Background()
	_, err := client.Audio(ctx, 1)
	assert.NoError(t, err)
}
