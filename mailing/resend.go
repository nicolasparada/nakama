package mailing

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v2"
)

type Resend struct {
	client *resend.Client
	from   string
}

func NewResend(from, apiKey string) *Resend {
	client := resend.NewClient(apiKey)
	return &Resend{
		client: client,
		from:   from,
	}
}

func (r *Resend) Send(ctx context.Context, to, subject, html, text string) error {
	_, err := r.client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From:    r.from,
		To:      []string{to},
		Subject: subject,
		Html:    html,
		Text:    text,
		ReplyTo: r.from,
	})
	if err != nil {
		return fmt.Errorf("resend: failed to send email to %q: %w", to, err)
	}
	return nil
}
