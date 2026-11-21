package mail

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// SESMailer sends through the SES v2 API. From e.g. "RelayForge <sign-in@relayforge.site>".
type SESMailer struct {
	Client *sesv2.Client
	From   string
}

func buildSendEmailInput(from string, m Message) *sesv2.SendEmailInput {
	return &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(from),
		Destination:      &types.Destination{ToAddresses: []string{m.To}},
		Content: &types.EmailContent{Simple: &types.Message{
			Subject: &types.Content{Data: aws.String(m.Subject), Charset: aws.String("UTF-8")},
			Body:    &types.Body{Text: &types.Content{Data: aws.String(m.Text), Charset: aws.String("UTF-8")}},
		}},
	}
}

func (s *SESMailer) Send(ctx context.Context, m Message) error {
	_, err := s.Client.SendEmail(ctx, buildSendEmailInput(s.From, m))
	return err
}
