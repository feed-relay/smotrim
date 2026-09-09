package media

import "context"

type EmptySizer struct{}

func (*EmptySizer) Sizes(ctx context.Context, urls []string) map[string]int64 {
	result := make(map[string]int64, len(urls))
	for _, url := range urls {
		result[url] = 0
	}
	return result
}
