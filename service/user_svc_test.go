package service

import (
	"reflect"
	"testing"
)

func Test_extractMentions(t *testing.T) {
	tt := []struct {
		name string
		in   string
		want []string
	}{
		// Basic functionality
		{
			name: "single_mention_at_start",
			in:   "@john hello there",
			want: []string{"john"},
		},
		{
			name: "single_mention_in_middle",
			in:   "hello @john how are you?",
			want: []string{"john"},
		},
		{
			name: "single_mention_at_end",
			in:   "hello there @john",
			want: []string{"john"},
		},
		{
			name: "multiple_mentions",
			in:   "@alice and @bob are friends",
			want: []string{"alice", "bob"},
		},
		{
			name: "duplicate_mentions",
			in:   "@john said hello to @john again",
			want: []string{"john"},
		},
		{
			name: "no_mentions",
			in:   "hello world without mentions",
			want: nil,
		},
		{
			name: "empty_string",
			in:   "",
			want: nil,
		},

		// Username format validation
		{
			name: "username_with_underscore",
			in:   "hello @user_name",
			want: []string{"user_name"},
		},
		{
			name: "username_with_dots",
			in:   "hello @user.name",
			want: []string{"user.name"},
		},
		{
			name: "username_with_hyphens",
			in:   "hello @user-name",
			want: []string{"user-name"},
		},
		{
			name: "username_with_numbers",
			in:   "hello @user123",
			want: []string{"user123"},
		},
		{
			name: "username_starting_with_number",
			in:   "hello @123user",
			want: []string{"123user"},
		},
		{
			name: "complex_username",
			in:   "hello @user123.test-name_",
			want: []string{"user123.test-name_"},
		},

		// Edge cases - should NOT match
		{
			name: "email_addresses_should_not_match",
			in:   "contact me at user@example.com please",
			want: nil,
		},
		{
			name: "multiple_email_addresses",
			in:   "emails: alice@test.com and bob@example.org",
			want: nil,
		},
		{
			name: "mention_in_email_should_not_match",
			in:   "send to @user@example.com",
			want: nil,
		},
		{
			name: "username_cannot_start_with_special_char",
			in:   "hello @.username",
			want: nil,
		},
		{
			name: "username_cannot_start_with_underscore",
			in:   "hello @_username",
			want: nil,
		},
		{
			name: "username_cannot_start_with_hyphen",
			in:   "hello @-username",
			want: nil,
		},

		// Context-based matching
		{
			name: "mention_after_space",
			in:   "hello @john there",
			want: []string{"john"},
		},
		{
			name: "mention_after_newline",
			in:   "hello\n@john",
			want: []string{"john"},
		},
		{
			name: "mention_after_tab",
			in:   "hello\t@john",
			want: []string{"john"},
		},
		{
			name: "mention_after_comma",
			in:   "hello,@john",
			want: []string{"john"},
		},
		{
			name: "mention_after_period",
			in:   "hello.@john",
			want: []string{"john"},
		},
		{
			name: "mention_after_exclamation",
			in:   "hello!@john",
			want: []string{"john"},
		},
		{
			name: "mention_after_question_mark",
			in:   "hello?@john",
			want: []string{"john"},
		},
		{
			name: "mention_after_semicolon",
			in:   "hello;@john",
			want: []string{"john"},
		},
		{
			name: "mention_after_colon",
			in:   "hello:@john",
			want: []string{"john"},
		},
		{
			name: "mention_after_parentheses",
			in:   "hello(@john) and (@alice)",
			want: []string{"john", "alice"},
		},
		{
			name: "mention_after_square_brackets",
			in:   "users[@john,@alice]",
			want: []string{"john", "alice"},
		},
		{
			name: "mention_after_curly_braces",
			in:   "data{@john:@alice}",
			want: []string{"john", "alice"},
		},

		// Real-world scenarios
		{
			name: "social_media_post",
			in:   "@alice just shared an amazing photo! @bob you should see this 📸",
			want: []string{"alice", "bob"},
		},
		{
			name: "conversation",
			in:   "Hey @john, did you see what @mary posted? CC: @admin",
			want: []string{"john", "mary", "admin"},
		},
		{
			name: "markdown_like_format",
			in:   "Thanks [@contributor](profile) and @maintainer for the help!",
			want: []string{"contributor", "maintainer"},
		},
		{
			name: "list_format",
			in:   "Team members: @alice, @bob, @charlie, and @diana",
			want: []string{"alice", "bob", "charlie", "diana"},
		},
		{
			name: "mixed_with_emails",
			in:   "Contact @support for help or email support@company.com",
			want: []string{"support"},
		},
		{
			name: "mixed_punctuation",
			in:   "Great work @team! Questions? Ask @lead. Issues? Contact @admin.",
			want: []string{"team", "lead", "admin"},
		},

		// Long content
		{
			name: "long_text_with_multiple_mentions",
			in: `This is a long post about our project. @alice started the initial design,
			@bob implemented the backend, @charlie handled the frontend, and @diana 
			managed the deployment. Special thanks to @mentor for guidance throughout.
			For questions, contact @support or @admin.`,
			want: []string{"alice", "bob", "charlie", "diana", "mentor", "support", "admin"},
		},

		// Boundary cases
		{
			name: "mention_at_very_beginning",
			in:   "@user",
			want: []string{"user"},
		},
		{
			name: "only_mention",
			in:   "@user",
			want: []string{"user"},
		},
		{
			name: "mention_with_trailing_punctuation",
			in:   "Hello @user!",
			want: []string{"user"},
		},
		{
			name: "mention_with_trailing_comma",
			in:   "Hello @user, how are you?",
			want: []string{"user"},
		},

		// Unicode and special characters
		{
			name: "mention_after_unicode_space",
			in:   "hello\u00A0@john", // non-breaking space
			want: []string{"john"},
		},
		{
			name: "multiple_spaces_before_mention",
			in:   "hello   @john",
			want: []string{"john"},
		},

		// Potential false positives that should not match
		{
			name: "at_symbol_in_middle_of_word",
			in:   "user@domain should not match",
			want: nil,
		},
		{
			name: "at_with_no_username",
			in:   "hello @ there",
			want: nil,
		},
		{
			name: "at_at_end",
			in:   "hello @",
			want: nil,
		},
		{
			name: "at_with_only_special_chars",
			in:   "hello @._-",
			want: nil,
		},

		// Case sensitivity
		{
			name: "mixed_case_usernames",
			in:   "@John and @ALICE and @bOb",
			want: []string{"John", "ALICE", "bOb"},
		},

		// Maximum username scenarios
		{
			name: "very_long_username_should_not_match",
			in:   "@verylongusernamethatmightexceedlimits",
			want: nil,
		},
		{
			name: "username_at_max_length",
			in:   "@abcdefghijklmnopqrstu", // exactly 21 characters
			want: []string{"abcdefghijklmnopqrstu"},
		},
		{
			name: "username_exceeding_max_length",
			in:   "@abcdefghijklmnopqrstuv", // 22 characters, should not match
			want: nil,
		},
		{
			name: "mixed_valid_and_invalid_length_usernames",
			in:   "@alice @verylongusernamethatexceedslimit @bob",
			want: []string{"alice", "bob"}, // only valid length usernames
		},
		{
			name: "username_with_unicode_characters",
			in:   "@café",         // 4 unicode characters, but only ASCII part will match
			want: []string{"caf"}, // only ASCII characters are captured
		},
		{
			name: "single_character_username",
			in:   "@a @b @c",
			want: []string{"a", "b", "c"},
		},

		// Nested structures
		{
			name: "mentions_in_quotes",
			in:   `"@alice said something" replied @bob`,
			want: []string{"alice", "bob"},
		},
		{
			name: "mentions_in_different_sentence_structures",
			in:   "(@alice) [@bob] {@charlie} <@diana>",
			want: []string{"alice", "bob", "charlie", "diana"},
		},

		// Additional punctuation and symbols
		{
			name: "mention_after_angle_brackets",
			in:   "Hello<@john> and <@alice>",
			want: []string{"john", "alice"},
		},
		{
			name: "mention_after_quotes",
			in:   `"@alice" '@bob' "@charlie"`,
			want: []string{"alice", "bob", "charlie"},
		},
		{
			name: "mention_after_slash",
			in:   "see/@john or check\\@alice",
			want: []string{"john", "alice"},
		},
		{
			name: "mention_after_pipe",
			in:   "users|@john|@alice",
			want: []string{"john", "alice"},
		},
		{
			name: "mention_after_asterisk",
			in:   "important*@admin please check",
			want: []string{"admin"},
		},
		{
			name: "mention_after_plus",
			in:   "result+@john and total+@alice",
			want: []string{"john", "alice"},
		},
		{
			name: "hyphen_before_mention_should_not_match",
			in:   "user-@alice should not match because hyphen is part of usernames",
			want: nil,
		},
		{
			name: "mention_after_equals",
			in:   "assigned=@john and owner=@alice",
			want: []string{"john", "alice"},
		},
		{
			name: "mention_after_ampersand",
			in:   "team&@john working with&@alice",
			want: []string{"john", "alice"},
		},
		{
			name: "mention_after_percent",
			in:   "progress%@john at 100%@alice",
			want: []string{"john", "alice"},
		},
		{
			name: "mention_after_hash",
			in:   "issue#@john or tag#@alice",
			want: []string{"john", "alice"},
		},
		{
			name: "mention_after_tilde",
			in:   "approx~@john or similar~@alice",
			want: []string{"john", "alice"},
		},
		{
			name: "mention_after_caret",
			in:   "version^@john or ref^@alice",
			want: []string{"john", "alice"},
		},

		// Smart/curly quotes and unicode punctuation
		{
			name: "mention_after_smart_quotes",
			in:   `"@alice" '@bob' "@charlie" '@diana'`,
			want: []string{"alice", "bob", "charlie", "diana"},
		},
		{
			name: "mention_after_em_dash",
			in:   "important—@admin please check",
			want: []string{"admin"},
		},
		{
			name: "mention_after_en_dash",
			in:   "range–@john to handle",
			want: []string{"john"},
		},
		{
			name: "mention_after_ellipsis",
			in:   "waiting…@alice will respond soon",
			want: []string{"alice"},
		},
		{
			name: "mention_after_bullet_points",
			in:   "• @john\n• @alice\n‣ @bob",
			want: []string{"john", "alice", "bob"},
		},
		{
			name: "mention_after_guillemets",
			in:   "French: « @alice » and ‹ @bob ›",
			want: []string{"alice", "bob"},
		},
		{
			name: "mention_after_section_symbol",
			in:   "See § @admin for details",
			want: []string{"admin"},
		},
		{
			name: "mention_after_copyright_trademark",
			in:   "© @company and ™ @brand",
			want: []string{"company", "brand"},
		},
		{
			name: "real_world_smart_punctuation_mix",
			in:   `"Hey @john, can you check this?" she asked. He replied: "Sure @alice—I'll look at it…"`,
			want: []string{"john", "alice"},
		},
		{
			name: "real_world_smart_punctuation_mix_normal_dash",
			in:   `"Hey @john, can you check this?" she asked. He replied: "Sure @alice-I'll look at it…"`,
			want: []string{"john", "alice-I"},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := extractMentions(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extractMentions() = %v, want %v", got, tc.want)
			}
		})
	}
}
