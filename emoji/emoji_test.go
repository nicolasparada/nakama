package emoji

import (
	"testing"
)

func TestIsValid(t *testing.T) {
	tt := []struct {
		name  string
		emoji string
		want  bool
	}{
		// Valid emojis (should be in the Unicode dataset)
		{"simple_smiley", "😀", true},
		{"heart", "❤️", true},
		{"thumbs_up", "👍", true},
		{"fire", "🔥", true},
		{"flag", "🇺🇸", true},
		{"skin_tone_modifier", "👍🏽", true},
		{"complex_family", "👨‍👩‍👧‍👦", true},
		{"woman_technologist", "👩‍💻", true},

		// Invalid cases
		{"empty_string", "", false},
		{"plain_text", "hello", false},
		{"numbers", "123", false},
		{"multiple_emojis", "😀😂", false},
		{"emoji_with_text", "😀 hello", false},
		{"text_with_emoji", "hello 😀", false},
		{"space_separated_emojis", "😀 😂", false},
		{"invalid_utf8", "\xff\xfe", false},
		{"special_chars", "!@#$", false},

		// Valid emojis
		{"simple_smiley", "😀", true},
		{"heart", "❤️", true},
		{"thumbs_up", "👍", true},
		{"fire", "🔥", true},
		{"flag", "🇺🇸", true},
		{"skin_tone_modifier", "👍🏽", true},
		{"complex_emoji", "👨‍👩‍👧‍👦", true},

		// recent emojis
		{"face_with_bags_under_eyes", "🥱", true},
		{"leafy_greens", "🥬", true},
		{"teapot", "🫖", true},
		{"people_hugging", "🫂", true},
		{"anatomical_heart", "🫀", true},
		{"lungs", "🫁", true},
		{"ninja", "🥷", true},
		{"person_in_tuxedo", "🤵", true},
		{"woman_in_tuxedo", "🤵‍♀️", true},
		{"man_in_tuxedo", "🤵‍♂️", true},
		{"mx_claus", "🧑‍🎄", true},
		{"rock", "🪨", true},
		{"wood", "🪵", true},
		{"hut", "🛖", true},
		{"pickup_truck", "🛻", true},
		{"roller_skate", "🛼", true},
		{"magic_wand", "🪄", true},
		{"pinata", "🪅", true},
		{"nesting_dolls", "🪆", true},
		{"sewing_needle", "🪡", true},
		{"knot", "🪢", true},
		{"melting_face", "🫠", true},
		{"saluting_face", "🫡", true},
		{"dotted_line_face", "🫥", true},
		{"face_with_open_eyes_and_hand_over_mouth", "🫢", true},
		{"face_with_peeking_eye", "🫣", true},
		{"biting_lip", "🫦", true},
		{"beans", "🫘", true},
		{"jar", "🫙", true},
		{"wheel", "🛞", true},
		{"ring_buoy", "🛟", true},
		{"hamsa", "🪬", true},
		{"mirror_ball", "🪩", true},
		{"low_battery", "🪫", true},

		{"multiple_emojis", "😀😂", false},

		// Invalid inputs
		{"empty_string", "", false},
		{"plain_text", "hello", false},
		{"numbers", "123", false},
		{"special_chars", "!@#", false},
		{"just_spaces", "   ", false},
		{"invalid_utf8", "\xff\xfe", false},

		// Mixed text and emoji test cases (all should be invalid)
		{"emoji_at_start", "😀hello", false},
		{"emoji_at_end", "hello😀", false},
		{"emoji_in_middle", "hel😀lo", false},
		{"emoji_with_numbers", "😀123", false},
		{"numbers_with_emoji", "123😀", false},
		{"emoji_with_punctuation", "😀!", false},
		{"punctuation_with_emoji", "!😀", false},
		{"emoji_separated_by_space", "😀 hello", false},
		{"text_separated_by_space", "hello 😀", false},
		{"emoji_space_emoji", "😀 😂", false},
		{"multiple_words_with_emoji", "hello 😀 world", false},
		{"emoji_between_words", "hello😀world", false},
		{"emoji_with_newline", "😀\ntext", false},
		{"text_with_newline_emoji", "text\n😀", false},
		{"emoji_with_tab", "😀\ttext", false},
		{"mixed_complex_emoji", "hello👨‍👩‍👧‍👦world", false},
		{"emoji_with_underscore", "😀_test", false},
		{"emoji_with_dash", "😀-test", false},
		{"parentheses_with_emoji", "(😀)", false},
		{"brackets_with_emoji", "[😀]", false},
		{"quotes_with_emoji", "\"😀\"", false},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := IsValid(tc.emoji)
			if got != tc.want {
				t.Errorf("IsValid(%q) = %v, want %v", tc.emoji, got, tc.want)
			}
		})
	}
}
