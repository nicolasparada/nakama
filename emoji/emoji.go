package emoji

import (
	"strings"
	"unicode/utf8"

	"github.com/kyokomi/emoji/v2"
)

var emojiMap = func() map[string]struct{} {
	out := map[string]struct{}{}
	for emoji := range emoji.RevCodeMap() {
		out[emoji] = struct{}{}
	}
	return out
}()

var yellowSkinToneMod = func() string {
	r, _ := utf8.DecodeRuneInString("\ufe0f")
	return string(r)
}()

func IsValid(s string) bool {
	_, ok := emojiMap[s]
	if !ok && strings.Contains(s, yellowSkinToneMod) {
		return IsValid(
			strings.ReplaceAll(s, yellowSkinToneMod, ""),
		)
	}
	return ok
}
