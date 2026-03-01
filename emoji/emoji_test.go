package emoji

import (
	"testing"
)

func Test_IsValid(t *testing.T) {
	tt := []struct {
		name  string
		emoji string
		want  bool
	}{
		{
			name:  "picker_thumbs_up",
			emoji: "👍️",
			want:  true,
		},
		{
			name:  "os_thumbs_up",
			emoji: "👍",
			want:  true,
		},
		{
			name:  "nope",
			emoji: "x",
			want:  false,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := IsValid(tc.emoji)
			if tc.want != got {
				t.Errorf("%q want %v; got %v", tc.emoji, tc.want, got)
			}
		})
	}
}
