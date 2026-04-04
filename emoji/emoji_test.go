package emoji

import "testing"

func Test_IsValid(t *testing.T) {
	tt := []struct {
		name  string
		emoji string
		want  bool
	}{
		{
			name:  "fully_qualified",
			emoji: "👍",
			want:  true,
		},
		{
			name:  "component",
			emoji: "🏻",
			want:  true,
		},
		{
			name:  "two_emojis",
			emoji: "👍😀",
			want:  false,
		},
		{
			name:  "emoji_then_text",
			emoji: "👍ok",
			want:  false,
		},
		{
			name:  "text_then_emoji",
			emoji: "ok👍",
			want:  false,
		},
		{
			name:  "minimally_qualified",
			emoji: "😶‍🌫",
			want:  false,
		},
		{
			name:  "empty",
			emoji: "",
			want:  false,
		},
		{
			name:  "invalid",
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
