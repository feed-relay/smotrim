package provider

import "time"

type Config interface {
	HTTPTimeout() time.Duration

	TestData() bool

	// ItunesOwnerName returns the name published in the <itunes:owner> tag.
	ItunesOwnerName() string

	// ItunesOwnerEmail returns the email published in the <itunes:owner> tag.
	ItunesOwnerEmail() string

	// Generator returns the value published in the <generator> tag,
	// identifying the software that produced the feed.
	Generator() string
}
