package accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type Email struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Mailer sends transactional email. The Resend implementation is used when
// RESEND_API_KEY is set; otherwise LogMailer prints emails to the server log so
// local setups still work (the inviter also gets the link to share by hand).
type Mailer interface {
	Send(ctx context.Context, email Email) error
}

var ErrMailerNotConfigured = errors.New("email isn't configured on this server (set RESEND_API_KEY); share the link instead")

// MailerFromEnv picks the mailer from the environment:
//
//	RESEND_API_KEY     enables sending through Resend
//	AYCORN_EMAIL_FROM  the From header (default: Resend's shared test sender)
func MailerFromEnv() Mailer {
	key := os.Getenv("RESEND_API_KEY")
	if key == "" {
		return LogMailer{}
	}
	from := os.Getenv("AYCORN_EMAIL_FROM")
	if from == "" {
		// Resend's shared sender only delivers to the Resend account's own
		// address; set AYCORN_EMAIL_FROM to a verified domain to invite others.
		from = "Aycorn <onboarding@resend.dev>"
	}
	return &ResendMailer{APIKey: key, From: from, Client: &http.Client{Timeout: 10 * time.Second}}
}

type LogMailer struct{}

func (LogMailer) Send(_ context.Context, email Email) error {
	log.Printf("email (not sent — no RESEND_API_KEY) to=%s subject=%q\n%s", email.To, email.Subject, email.Text)
	return ErrMailerNotConfigured
}

type ResendMailer struct {
	APIKey string
	From   string
	Client *http.Client
}

func (m *ResendMailer) Send(ctx context.Context, email Email) error {
	payload, err := json.Marshal(map[string]any{
		"from":    m.From,
		"to":      []string{email.To},
		"subject": email.Subject,
		"text":    email.Text,
		"html":    email.HTML,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := m.Client.Do(req)
	if err != nil {
		return fmt.Errorf("sending email: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("resend rejected the email (%d): %s", res.StatusCode, bytes.TrimSpace(body))
	}
	return nil
}
