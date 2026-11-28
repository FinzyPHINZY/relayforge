package mail

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

type SESMailer struct {
	Client *sesv2.Client
	From   string
}

func buildSendEmailInput(from string, m Message) *sesv2.SendEmailInput {
	body := &types.Body{Text: &types.Content{Data: aws.String(m.Text), Charset: aws.String("UTF-8")}}
	if m.HTML != "" {
		body.Html = &types.Content{Data: aws.String(m.HTML), Charset: aws.String("UTF-8")}
	}
	return &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(from),
		Destination:      &types.Destination{ToAddresses: []string{m.To}},
		Content: &types.EmailContent{Simple: &types.Message{
			Subject: &types.Content{Data: aws.String(m.Subject), Charset: aws.String("UTF-8")},
			Body:    body,
		}},
	}
}

func (s *SESMailer) Send(ctx context.Context, m Message) error {
	_, err := s.Client.SendEmail(ctx, buildSendEmailInput(s.From, m))
	return err
}
