package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/vision"
)

// Mailer sends one plain-text message. The host uses it for email
// confirmation codes and invitations; with none configured, email is simply
// not offered.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// sesMailer sends through Amazon SES with the container's own role (or keys
// in the environment), signed by hand: the image has no SDK and no shell.
type sesMailer struct {
	region, from string
	creds        *vision.CredentialSource
	http         *http.Client
}

// MailerFromEnv returns an SES sender when LAMDIS_MAIL_FROM and a region are
// set, else nil.
func MailerFromEnv() Mailer {
	from := strings.TrimSpace(os.Getenv("LAMDIS_MAIL_FROM"))
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		region = strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
	}
	if from == "" || region == "" {
		return nil
	}
	return &sesMailer{region: region, from: from, creds: &vision.CredentialSource{}, http: &http.Client{Timeout: 15 * time.Second}}
}

func (s *sesMailer) Send(ctx context.Context, to, subject, body string) error {
	creds, err := s.creds.Get(ctx)
	if err != nil {
		return fmt.Errorf("mail: no credentials: %w", err)
	}
	form := url.Values{}
	form.Set("Action", "SendEmail")
	form.Set("Version", "2010-12-01")
	form.Set("Source", s.from)
	form.Set("Destination.ToAddresses.member.1", to)
	form.Set("Message.Subject.Data", subject)
	form.Set("Message.Body.Text.Data", body)
	payload := []byte(form.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://email."+s.region+".amazonaws.com/", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	vision.SignV4(req, payload, creds, s.region, "ses", time.Now())
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("mail: ses returned %d", resp.StatusCode)
	}
	return nil
}
