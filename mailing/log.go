package mailing

import (
	"context"
	"fmt"
	"net/mail"
)

// LogSender log emails.
type LogSender struct {
	From mail.Address
}

// NewLogSender implementation using the provided logger.
func NewLogSender(from string) *LogSender {
	return &LogSender{
		From: mail.Address{Name: "nakama", Address: from},
	}
}

// Send will just log the email.
func (s *LogSender) Send(_ context.Context, to, subject, html, text string) error {
	toAddr := mail.Address{Address: to}
	b, err := buildBody(s.From, toAddr, subject, html, text)
	if err != nil {
		return err
	}

	fmt.Println("mailing:\n", string(b))
	return nil
}
