package mocks

// FakeFeed is a hand-written fixture, not a moq mock: Feed /
// contracts.Feed is three plain getters with no call-tracking
// worth mocking, so a literal struct reads clearer here. NOTE: this
// assumes contracts.Feed needs exactly these three methods
// (matching provider.Feed) - if the real interface has more, add stub
// implementations for those too.
type FakeFeed struct {
	RawPerShowLimit int
	RawShows        []string
	RawSlug         string
	RawLink         string
	RawTitle        string
	RawDescription  string
	RawImage        string
}

func (s FakeFeed) Limit() int          { return s.RawPerShowLimit }
func (s FakeFeed) Shows() []string     { return s.RawShows }
func (s FakeFeed) Slug() string        { return s.RawSlug }
func (s FakeFeed) Link() string        { return s.RawLink }
func (s FakeFeed) Title() string       { return s.RawTitle }
func (s FakeFeed) Description() string { return s.RawDescription }
func (s FakeFeed) Image() string       { return s.RawImage }
