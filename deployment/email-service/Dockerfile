# Build stage
FROM golang:1.22.1 as build-stage
WORKDIR /app
COPY go.mod go.sum Makefile .env ./
COPY cmd/email-service/worker ./worker/
COPY cmd/email-service/client ./client/
COPY internal ./internal/
RUN go mod download

# Client build stage
FROM build-stage as client-build-stage
RUN CGO_ENABLED=0 GOOS=linux go build -o /client ./client

# Worker build stage
FROM build-stage as worker-build-stage
RUN CGO_ENABLED=0 GOOS=linux go build -o /worker ./worker

# Client release stage
FROM golang:1.22.1 AS client-release-stage
WORKDIR /
COPY --from=client-build-stage /client /client
COPY --from=build-stage /app/internal /internal
COPY --from=build-stage /app/.env /.env
EXPOSE 8080
ENTRYPOINT ["/client"]

# Worker release stage
FROM golang:1.22.1 AS worker-release-stage
WORKDIR /
COPY --from=worker-build-stage /worker /worker
COPY --from=build-stage /app/internal /internal
COPY --from=build-stage /app/.env /.env
EXPOSE 8081
ENTRYPOINT ["/worker"]