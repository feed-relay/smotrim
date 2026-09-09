package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractNumber(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:    "plain number",
			input:   "12344",
			want:    12344,
			wantErr: false,
		},
		{
			name:    "url with number",
			input:   "https://example.com/section/12345",
			want:    12345,
			wantErr: false,
		},
		{
			name:    "url with number and trailing slash",
			input:   "https://example.com/section/12346/",
			want:    12346,
			wantErr: false,
		},
		{
			name:    "domain with number",
			input:   "example.com/section/12347",
			want:    12347,
			wantErr: false,
		},
		{
			name:    "domain with number and multiple trailing slashes",
			input:   "example.com/section/12348//",
			want:    12348,
			wantErr: false,
		},
		{
			name:    "number with trailing spaces",
			input:   "12349   ",
			want:    12349,
			wantErr: false,
		},
		{
			name:    "multiple numbers in string (gets the last one)",
			input:   "https://example.com/99/section/100/",
			want:    100,
			wantErr: false,
		},
		{
			name:    "no digits present",
			input:   "https://example.com/section/abc",
			want:    0,
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			want:    0,
			wantErr: true,
		},
		{
			name:    "only slashes",
			input:   "///",
			want:    0,
			wantErr: true,
		},
		{
			name:    "number overflow (exceeds int capacity)",
			input:   "999999999999999999999",
			want:    0,
			wantErr: true,
		},
		{
			name:    "zero",
			input:   "0",
			want:    0,
			wantErr: false,
		},
		// ================================================================
		{
			name:  "leading and trailing spaces",
			input: "  example.com/section/12349/  ",
			want:  12349,
		},
		{
			name:  "single path segment",
			input: "12350/",
			want:  12350,
		},
		{
			name:  "deep path",
			input: "example.com/foo/bar/baz/12351",
			want:  12351,
		},
		{
			name:  "large number",
			input: "9223372036854775807",
			want:  9223372036854775807,
		},
		{
			name:    "no number at the end",
			input:   "example.com/section/about",
			wantErr: true,
		},
		{
			name:    "non-numeric value",
			input:   "hello",
			wantErr: true,
		},
		{
			name:    "number followed by text",
			input:   "example.com/section/123abc",
			wantErr: true,
		},
		{
			name:  "negative number",
			input: "-123",
			want:  -123,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provider{}
			got, err := p.extractNumber(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
