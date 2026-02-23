package cursor

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

func Encode(key string, ts time.Time) string {
	s := fmt.Sprintf("%s,%s", key, ts.Format(time.RFC3339Nano))
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func EncodeSimple(key string) string {
	return base64.StdEncoding.EncodeToString([]byte(key))
}

func Decode(s string) (string, time.Time, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("could not base64 decode cursor: %w", err)
	}

	parts := strings.Split(string(b), ",")
	if len(parts) != 2 {
		return "", time.Time{}, errors.New("expected cursor to have two items split by comma")
	}

	ts, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil {
		return "", time.Time{}, fmt.Errorf("could not parse cursor timestamp: %w", err)
	}

	key := parts[0]
	return key, ts, nil
}

func DecodeSimple(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("could not base64 decode cursor: %w", err)
	}

	return string(b), nil
}
