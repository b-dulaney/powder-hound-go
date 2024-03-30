package email

import "github.com/resend/resend-go/v2"

type EmailData struct {
	Location string
	Snowfall int
}

type ResendService struct {
	Client *resend.Client
}

type MockEmailService struct{}

type EmailService interface {
	SendEmail(subject string, body string, to string) error
}
