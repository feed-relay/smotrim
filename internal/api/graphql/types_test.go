package graphql

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSmotrimTime(t *testing.T) {
	t.Run("MarshalJSON", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		st := SmotrimTime(now)
		b, err := json.Marshal(st)
		assert.NoError(t, err)

		var back time.Time
		err = json.Unmarshal(b, &back)
		assert.NoError(t, err)
		assert.True(t, now.Equal(back))
	})

	t.Run("Time method", func(t *testing.T) {
		now := time.Now()
		st := SmotrimTime(now)
		assert.True(t, now.Equal(st.Time()))
	})

	t.Run("UnmarshalJSON", func(t *testing.T) {
		tests := []struct {
			name string
			json string
			want time.Time
		}{
			{"RFC3339", `"2020-01-02T15:04:05Z"`, time.Date(2020, 1, 2, 15, 4, 5, 0, time.UTC)},
			{"Custom format", `"2020-01-02T15:04:05+0300"`, time.Date(2020, 1, 2, 15, 4, 5, 0, time.FixedZone("UTC+3", 3*3600))},
			{"Null", `null`, time.Time{}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var st SmotrimTime
				err := json.Unmarshal([]byte(tt.json), &st)
				assert.NoError(t, err)
				if tt.json == `null` {
					assert.True(t, st.Time().IsZero())
				} else {
					assert.True(t, tt.want.Equal(st.Time()))
				}
			})
		}
	})
}
