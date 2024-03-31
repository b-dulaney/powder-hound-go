# Build stage
FROM golang:1.22.1 as build-stage
WORKDIR /app
COPY go.mod go.sum Makefile .env ./
COPY config ./config/
COPY cmd/scraping-service/worker ./worker/
COPY cmd/scraping-service/client ./client/
COPY internal ./internal/
RUN go mod download

# Client build stage
FROM build-stage as client-build-stage
RUN CGO_ENABLED=0 GOOS=linux go build -o /client ./client

# Worker build stage
FROM build-stage as worker-build-stage
RUN CGO_ENABLED=0 GOOS=linux go build -o /worker ./worker

# Client release stage
FROM chromedp/headless-shell:114.0.5735.199 AS client-release-stage
RUN apt-get update && apt-get install -y ca-certificates --no-install-recommends && rm -rf /var/lib/apt/lists/*
WORKDIR /
COPY --from=client-build-stage /client /client
COPY --from=build-stage /app/config /config
COPY --from=build-stage /app/internal /internal
COPY --from=build-stage /app/.env /.env
EXPOSE 8080
ENTRYPOINT ["/client"]

# Worker release stage
FROM chromedp/headless-shell:114.0.5735.199 AS worker-release-stage
RUN apt-get update && apt-get install -y ca-certificates --no-install-recommends && rm -rf /var/lib/apt/lists/*
WORKDIR /
COPY --from=worker-build-stage /worker /worker
COPY --from=build-stage /app/config /config
COPY --from=build-stage /app/internal /internal
COPY --from=build-stage /app/.env /.env
EXPOSE 8081
ENTRYPOINT ["/worker"]