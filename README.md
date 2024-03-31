![Logo](https://powderhound-static-images.s3.us-east-2.amazonaws.com/logo-256px.png)

# PowderHound-Go

PowderHound-Go is a collection of backend services for PowderHound, primarily handling email delivery and web scraping jobs. These services are built using a distributed task architecture leveraging the Go [Asynq](https://github.com/hibiken/asynq) library.

## Structure

The project is structured as follows:

- [`cmd/`](command:_github.copilot.openRelativePath?%5B%22cmd%2F%22%5D "cmd/"): Contains the main applications for the project (email-service and scraping-service).
- [`config/`](command:_github.copilot.openRelativePath?%5B%22config%2F%22%5D "config/"): Contains JSON configuration files that control the resort web scraping jobs.
- [`deployment/`](command:_github.copilot.openRelativePath?%5B%22deployment%2F%22%5D "deployment/"): Contains Docker files for the email and scraping services.
- [`internal/`](command:_github.copilot.openRelativePath?%5B%22internal%2F%22%5D "internal/"): Internal packages that contain most of the logic for the email and web scraping services.

## Services

### Email Service

The Email Service is responsible for building and sending forecast and overnight alert emails. It uses the [Hermes](https://github.com/matcornic/hermes) library for building the emails and [Resend](https://resend.com/overview) for delivery. The main logic can be found in [`internal/email/email.go`](command:_github.copilot.openSymbolInFile?%5B%22internal%2Femail%2Femail.go%22%2C%22internal%2Femail%2Femail.go%22%5D "internal/email/email.go").

### Scraping Service

The Scraping Service is responsible for scraping ski resort data from various resort websites. It uses the Chromedp library for web scraping. The main logic can be found in [`internal/scraping/scraping.go`](command:_github.copilot.openSymbolInFile?%5B%22internal%2Fscraping%2Fscraping.go%22%2C%22internal%2Fscraping%2Fscraping.go%22%5D "internal/scraping/scraping.go").

## Deployment

Both services are containerized using Docker, and are deployed via GitHub Actions using the provided Docker Compose files in the [`deployment/`](command:_github.copilot.openRelativePath?%5B%22deployment%2F%22%5D "deployment/") directory.
