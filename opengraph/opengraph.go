package opengraph

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type OpenGraph struct {
	// Essential properties for link previews
	Title       string
	Description string
	URL         string
	SiteName    string
	Type        string

	// Images for rich previews
	Images []Image
}

func (og OpenGraph) IsEmpty() bool {
	return og.Title == "" && og.Description == "" && og.SiteName == "" && len(og.Images) == 0
}

type Image struct {
	URL       string
	SecureURL string
	Width     uint32
	Height    uint32
	Alt       string
	Type      string
}

func Parse(r io.Reader) (OpenGraph, error) {
	var out OpenGraph

	node, err := html.Parse(r)
	if err != nil {
		return out, fmt.Errorf("parse HTML: %w", err)
	}

	extractOGTags(node, &out)
	return out, nil
}

// extractOGTags recursively walks the HTML tree and extracts OpenGraph properties
func extractOGTags(n *html.Node, og *OpenGraph) {
	if n.Type == html.ElementNode && n.Data == "meta" {
		var property, content string

		for _, attr := range n.Attr {
			switch attr.Key {
			case "property":
				property = attr.Val
			case "content":
				content = attr.Val
			}
		}

		if strings.HasPrefix(property, "og:") {
			parseOGProperty(property, content, og)
		}
	}

	// Recursively process child nodes
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractOGTags(c, og)
	}
}

// parseOGProperty parses OpenGraph properties relevant for link previews
func parseOGProperty(property, content string, og *OpenGraph) {
	switch property {
	// Basic properties
	case "og:title":
		og.Title = content
	case "og:description":
		og.Description = content
	case "og:url":
		og.URL = content
	case "og:site_name":
		og.SiteName = content
	case "og:type":
		og.Type = content

	// Image properties
	case "og:image":
		og.Images = append(og.Images, Image{URL: content})
	case "og:image:url":
		if len(og.Images) > 0 {
			og.Images[len(og.Images)-1].URL = content
		} else {
			og.Images = append(og.Images, Image{URL: content})
		}
	case "og:image:secure_url":
		ensureLastImage(og)
		og.Images[len(og.Images)-1].SecureURL = content
	case "og:image:width":
		ensureLastImage(og)
		if width, err := strconv.ParseUint(content, 10, 32); err == nil {
			og.Images[len(og.Images)-1].Width = uint32(width)
		}
	case "og:image:height":
		ensureLastImage(og)
		if height, err := strconv.ParseUint(content, 10, 32); err == nil {
			og.Images[len(og.Images)-1].Height = uint32(height)
		}
	case "og:image:alt":
		ensureLastImage(og)
		og.Images[len(og.Images)-1].Alt = content
	case "og:image:type":
		ensureLastImage(og)
		og.Images[len(og.Images)-1].Type = content
	}
}

// ensureLastImage ensures we have at least one image before setting properties
func ensureLastImage(og *OpenGraph) {
	if len(og.Images) == 0 {
		og.Images = append(og.Images, Image{})
	}
}
