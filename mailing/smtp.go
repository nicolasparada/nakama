package mailing

import (
	"context"
	"fmt"
	"net/mail"
	"net/smtp"
)

// SMTPSender sends emails using an SMTP service.
type SMTPSender struct {
	From mail.Address
	Addr string
	Auth smtp.Auth
}

// NewSMTPSender implementation using an SMTP server.
func NewSMTPSender(from, host string, port int, username, password string) *SMTPSender {
	return &SMTPSender{
		From: mail.Address{Name: "nakama", Address: from},
		Addr: fmt.Sprintf("%s:%d", host, port),
		Auth: smtp.PlainAuth("", username, password, host),
	}
}

// Send an email to the given email address.
func (s *SMTPSender) Send(_ context.Context, to, subject, html, text string) error {
	toAddr := mail.Address{Address: to}
	b, err := buildBody(s.From, toAddr, subject, html, text)
	if err != nil {
		return err
	}

	return smtp.SendMail(
		s.Addr,
		s.Auth,
		s.From.Address,
		[]string{toAddr.Address},
		b,
	)
}
