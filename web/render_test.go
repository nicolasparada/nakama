package web

import (
	"testing"
)

func TestLinkify(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// URL tests
		{
			name:     "simple_https_url",
			input:    "Check out https://example.com for more info",
			expected: `Check out <a href="https://example.com" target="_blank" rel="noopener">https://example.com</a> for more info`,
		},
		{
			name:     "url_with_trailing_punctuation",
			input:    "See https://example.com.",
			expected: `See <a href="https://example.com" target="_blank" rel="noopener">https://example.com</a>.`,
		},
		{
			name:     "multiple_urls",
			input:    "Check https://site1.com and https://site2.com",
			expected: `Check <a href="https://site1.com" target="_blank" rel="noopener">https://site1.com</a> and <a href="https://site2.com" target="_blank" rel="noopener">https://site2.com</a>`,
		},

		// Mention tests
		{
			name:     "single_mention",
			input:    "Hello @john how are you?",
			expected: `Hello <a href="/u/john" class="primary">@john</a> how are you?`,
		},
		{
			name:     "multiple_mentions",
			input:    "@alice and @bob are friends",
			expected: `<a href="/u/alice" class="primary">@alice</a> and <a href="/u/bob" class="primary">@bob</a> are friends`,
		},
		{
			name:     "mention_with_punctuation",
			input:    "Great work @team! Questions? Ask @lead.",
			expected: `Great work <a href="/u/team" class="primary">@team</a>! Questions? Ask <a href="/u/lead" class="primary">@lead</a>.`,
		},
		{
			name:     "email_should_not_match",
			input:    "Contact me at user@example.com",
			expected: "Contact me at user@example.com",
		},
		{
			name:     "mention_in_email_should_not_match",
			input:    "Send to @user@example.com",
			expected: "Send to @user@example.com",
		},

		// Combined URL and mention tests
		{
			name:     "url_and_mention_combined",
			input:    "Hey @john, check out https://example.com!",
			expected: `Hey <a href="/u/john" class="primary">@john</a>, check out <a href="https://example.com" target="_blank" rel="noopener">https://example.com</a>!`,
		},
		{
			name:     "mention_then_url",
			input:    "@alice found this: https://github.com/user/repo.",
			expected: `<a href="/u/alice" class="primary">@alice</a> found this: <a href="https://github.com/user/repo" target="_blank" rel="noopener">https://github.com/user/repo</a>.`,
		},

		// Edge cases
		{
			name:     "empty_string",
			input:    "",
			expected: "",
		},
		{
			name:     "no_links_or_mentions",
			input:    "This has no links or mentions at all",
			expected: "This has no links or mentions at all",
		},
		{
			name:     "mention_after_smart_quotes",
			input:    `"Hey @john!" she said`,
			expected: `"Hey <a href="/u/john" class="primary">@john</a>!" she said`,
		},
		{
			name:     "very_long_username_should_not_match",
			input:    "@verylongusernamethatexceedslimit should not match",
			expected: "@verylongusernamethatexceedslimit should not match",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := linkify(tc.input)
			if string(result) != tc.expected {
				t.Errorf("linkify() = %q, want %q", string(result), tc.expected)
			}
		})
	}
}
