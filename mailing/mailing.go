//go:generate go tool moq -rm -stub -out sender_mock.go . Sender

package mailing

import (
	"bytes"
	"context"
	"fmt"
	"net/mail"

	mailutl "github.com/go-mail/mail"
)

// Sender sends mails.
type Sender interface {
	Send(ctx context.Context, to, subject, html, text string) error
}

func buildBody(from, to mail.Address, subject, html, text string) ([]byte, error) {
	m := mailutl.NewMessage()
	m.SetHeader("From", from.String())
	m.SetHeader("To", to.String())
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", html)
	m.AddAlternative("text/plain", text)

	buff := &bytes.Buffer{}
	_, err := m.WriteTo(buff)
	if err != nil {
		return nil, fmt.Errorf("could not build mail body: %w", err)
	}

	return buff.Bytes(), nil
}
