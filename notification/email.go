package notification

import (
	"summary-notion/config"

	"github.com/resend/resend-go/v2"
)

func SendEmail(subject string, to string, content string) error {
	resendClient := resend.NewClient(config.Email.APIKey)
	params := &resend.SendEmailRequest{
		From:    config.Email.FROM,
		To:      []string{to},
		Subject: subject,
		Html:    content,
	}

	_, err := resendClient.Emails.Send(params)
	return err
}

func SendMoreEmails(subject string, toSends map[string]string) error {
	for to, content := range toSends {
		err := SendEmail(subject, to, content)
		if err != nil {
			return err
		}
	}

	return nil
}
