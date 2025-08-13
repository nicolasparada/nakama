package opengraph

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tt := []struct {
		name string
		in   string
		want OpenGraph
	}{
		{
			name: "basic_properties",
			in: `<!DOCTYPE html>
<html>
<head>
	<meta property="og:title" content="Test Title">
	<meta property="og:description" content="Test Description">
	<meta property="og:url" content="https://example.com">
	<meta property="og:site_name" content="Example Site">
	<meta property="og:type" content="website">
</head>
<body></body>
</html>`,
			want: OpenGraph{
				Title:       "Test Title",
				Description: "Test Description",
				URL:         "https://example.com",
				SiteName:    "Example Site",
				Type:        "website",
			},
		},
		{
			name: "single_image",
			in: `<!DOCTYPE html>
<html>
<head>
	<meta property="og:title" content="Image Test">
	<meta property="og:image" content="https://example.com/image.jpg">
</head>
<body></body>
</html>`,
			want: OpenGraph{
				Title: "Image Test",
				Images: []Image{{
					URL: "https://example.com/image.jpg",
				}},
			},
		},
		{
			name: "multiple_images",
			in: `<!DOCTYPE html>
<html>
<head>
	<meta property="og:title" content="Multiple Images">
	<meta property="og:image" content="https://example.com/image1.jpg">
	<meta property="og:image:width" content="800">
	<meta property="og:image:height" content="600">
	<meta property="og:image" content="https://example.com/image2.jpg">
	<meta property="og:image:width" content="1200">
	<meta property="og:image:height" content="800">
</head>
<body></body>
</html>`,
			want: OpenGraph{
				Title: "Multiple Images",
				Images: []Image{
					{
						URL:    "https://example.com/image1.jpg",
						Width:  800,
						Height: 600,
					},
					{
						URL:    "https://example.com/image2.jpg",
						Width:  1200,
						Height: 800,
					},
				},
			},
		},
		{
			name: "twitter_example",
			in: `<!DOCTYPE html>
<html>
<head>
	<meta property="og:title" content="Twitter Card">
	<meta property="og:description" content="Sample tweet.">
	<meta property="og:url" content="https://twitter.com/user/status/123">
	<meta property="og:site_name" content="Twitter">
	<meta property="og:type" content="article">
	<meta property="og:image" content="https://pbs.twimg.com/media/sample.jpg">
	<meta property="og:image:width" content="1200">
	<meta property="og:image:height" content="675">
</head>
<body></body>
</html>`,
			want: OpenGraph{
				Title:       "Twitter Card",
				Description: "Sample tweet.",
				URL:         "https://twitter.com/user/status/123",
				SiteName:    "Twitter",
				Type:        "article",
				Images: []Image{{
					URL:    "https://pbs.twimg.com/media/sample.jpg",
					Width:  1200,
					Height: 675,
				}},
			},
		},
		{
			name: "empty_html",
			in:   ``,
			want: OpenGraph{},
		},
		{
			name: "no_og_tags",
			in: `<!DOCTYPE html>
<html>
<head>
	<title>Regular Title</title>
	<meta name="description" content="Regular description">
</head>
<body></body>
</html>`,
			want: OpenGraph{
				Title:       "Regular Title",
				Description: "Regular description",
			},
		},
		{
			name: "unicode_content",
			in: `<!DOCTYPE html>
<html>
<head>
	<meta property="og:title" content="Unicode: 🚀 测试">
	<meta property="og:description" content="Description with émojis 😀">
</head>
<body></body>
</html>`,
			want: OpenGraph{
				Title:       "Unicode: 🚀 测试",
				Description: "Description with émojis 😀",
			},
		},
		{
			name: "malformed_html",
			in: `<!DOCTYPE html>
<html>
<head>
	<meta property="og:title" content="Malformed Test">
	<meta property="og:description" content="Missing closing tag">
</head>
<body></body>
</html>`,
			want: OpenGraph{
				Title:       "Malformed Test",
				Description: "Missing closing tag",
			},
		},
		{
			name: "fallback_values",
			in: `<!DOCTYPE html>
<html>
<head>
	<title>Fallback Page Title</title>
	<meta name="description" content="This is a fallback description">
	<link rel="canonical" href="https://www.example.com/page">
</head>
<body></body>
</html>`,
			want: OpenGraph{
				Title:       "Fallback Page Title",
				Description: "This is a fallback description",
				URL:         "https://www.example.com/page",
			},
		},
	}

	// Test with domain name extraction for site name
	domainTestCases := []struct {
		name    string
		in      string
		baseURL string
		want    OpenGraph
	}{
		{
			name: "domain_site_name_fallback",
			in: `<!DOCTYPE html>
<html>
<head>
	<title>Page without OpenGraph</title>
</head>
<body></body>
</html>`,
			baseURL: "https://www.github.com/user/repo",
			want: OpenGraph{
				Title:    "Page without OpenGraph",
				SiteName: "Github.com",
			},
		},
	}

	for _, tc := range domainTestCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tc.in), tc.baseURL)
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Test %s failed", tc.name)
				t.Errorf("Want: %+v", tc.want)
				t.Errorf("Got:  %+v", got)
			}
		})
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tc.in), "")
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Test %s failed", tc.name)
				t.Errorf("Want: %+v", tc.want)
				t.Errorf("Got:  %+v", got)
			}
		})
	}
}
