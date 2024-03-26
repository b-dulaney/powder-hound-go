package tasks

import (
	"fmt"
	"log"

	"github.com/matcornic/hermes/v2"
)

var h = hermes.Hermes{
	Product: hermes.Product{
		Name:      "The PowderHound team",
		Link:      "https://powderhound.io",
		Logo:      "https://powderhound-static-images.s3.us-east-2.amazonaws.com/logo-256px.png",
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
						key: "25%",
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
