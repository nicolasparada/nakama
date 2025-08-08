package mailing

import (
	"context"
	"net/mail"

	"github.com/go-kit/log"
)

// LogSender log emails.
type LogSender struct {
	From   mail.Address
	Logger log.Logger
}

// NewLogSender implementation using the provided logger.
func NewLogSender(from string, l log.Logger) *LogSender {
	return &LogSender{
		From:   mail.Address{Name: "nakama", Address: from},
		Logger: l,
	}
}

// Send will just log the email.
func (s *LogSender) Send(_ context.Context, to, subject, html, text string) error {
	toAddr := mail.Address{Address: to}
	b, err := buildBody(s.From, toAddr, subject, html, text)
	if err != nil {
		return err
	}

	_ = s.Logger.Log("mail", string(b))
	return nil
}
