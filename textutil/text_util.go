package textutil

import (
	"regexp"
	"strings"
)

var (
	reMultiSpace          = regexp.MustCompile(`(\s)+`)
	reMoreThan2Linebreaks = regexp.MustCompile(`(\n){2,}`)
	reMentions            = regexp.MustCompile(`\B@([a-zA-Z][a-zA-Z0-9_-]{0,17})(?:\b[^@]|$)`)
	reTags                = regexp.MustCompile(`\B#((?:\p{L}|\p{N}|_)+)(?:\b[^#]|$)`)
)

func SmartTrim(s string) string {
	oldLines := strings.Split(s, "\n")
	newLines := []string{}
	for _, line := range oldLines {
		line = strings.TrimSpace(reMultiSpace.ReplaceAllString(line, "$1"))
		newLines = append(newLines, line)
	}
	s = strings.Join(newLines, "\n")
	s = reMoreThan2Linebreaks.ReplaceAllString(s, "$1$1")
	return strings.TrimSpace(s)
}

func CollectMentions(s string) []string {
	mentions := map[string]struct{}{}
	var unique []string
	for _, submatch := range reMentions.FindAllStringSubmatch(s, -1) {
		mention := submatch[1]
		if _, ok := mentions[mention]; !ok {
			mentions[mention] = struct{}{}
			unique = append(unique, mention)
		}
	}
	return unique
}

func CollectTags(s string) []string {
	tags := map[string]struct{}{}
	var unique []string
	for _, submatch := range reTags.FindAllStringSubmatch(s, -1) {
		tag := submatch[1]
		if _, ok := tags[tag]; !ok {
			tags[tag] = struct{}{}
			unique = append(unique, tag)
		}
	}
	return unique
}
