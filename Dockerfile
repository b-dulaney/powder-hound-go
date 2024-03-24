FROM golang:1.22.1

WORKDIR /app
COPY go.mod go.sum .env ./
COPY config ./config
RUN go mod download
COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /powder-hound-go
CMD ["/powder-hound-go"]
