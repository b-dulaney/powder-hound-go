package email

import (
	"fmt"
	"log"
	"os"

	"github.com/matcornic/hermes/v2"
	"github.com/resend/resend-go/v2"
)

var h = hermes.Hermes{
	Product: hermes.Product{
		Name:      "The PowderHound team",
		Link:      "https://powderhound.io",
		Logo:      "https://powderhound-static-images.s3.us-east-2.amazonaws.com/logo-256px.png?",
		Copyright: "❤️ powderhound.io",
	},
}

func BuildAlertEmail(emailData []EmailData, title, intro, key string) string {
	var tableData [][]hermes.Entry

	for _, data := range emailData {
		tableData = append(tableData, []hermes.Entry{
			{Key: "Location", Value: data.Location},
			{Key: key, Value: fmt.Sprintf("%v\"", data.Snowfall)},
		})
	}

	email := hermes.Email{
		Body: hermes.Body{
			Title:     title,
			Signature: "Cheers",
			Intros: []string{
				intro,
			},
			Table: hermes.Table{
				Data: tableData,
				Columns: hermes.Columns{
					CustomWidth: map[string]string{
						key: "40%",
					},
					CustomAlignment: map[string]string{
						key: "right",
					},
				},
			},
			Actions: []hermes.Action{
				{
					Instructions: "View the full snow report and more on PowderHound:",
					Button: hermes.Button{
						Text: "View Snow Report",
						Link: "https://powderhound.io/snow-report/resorts",
					},
				},
			},
		},
	}

	emailBody, err := h.GenerateHTML(email)
	if err != nil {
		log.Fatal(err)
	}

	return emailBody
}

func BuildForecastAlertEmail(emailData []EmailData) string {
	return BuildAlertEmail(emailData, "Upcoming Snowfall", "A forecast alert has been triggered for the following locations.", "Next 24 Hours")
}

func BuildOvernightAlertEmail(emailData []EmailData) string {
	return BuildAlertEmail(emailData, "Snowfall Alert", "The following locations have received fresh snowfall.", "Fresh Snow")
}

func (s *ResendService) SendEmail(subject string, body string, to string) error {
	params := &resend.SendEmailRequest{
		From:    "PowderHound <alerts@powderhound.io>",
		To:      []string{to},
		Subject: subject,
		Html:    body,
	}
	sent, err := s.Client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send %s email to %s: %w", subject, to, err)
	}

	log.Printf("Finished sending %s email to %s. Resend ID: %s", subject, to, sent.Id)
	return nil
}

func (s *MockEmailService) SendEmail(subject string, body string, to string) error {
	log.Printf("Mock email sent to %s with subject %s and body %s", to, subject, body)
	return nil
}

func NewResendService() EmailService {
	ENV := os.Getenv("ENV")
	API_KEY := os.Getenv("RESEND_API_KEY")

	if ENV == "production" {
		client := resend.NewClient(API_KEY)
		return &ResendService{
			Client: client,
		}
	}

	return &MockEmailService{}

}
