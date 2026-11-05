# Built by GitHub Actions and pushed to ghcr.io; secrets are passed at runtime (env_file),
# never baked into the image.
# Build stage
FROM golang:1.25.0 as build-stage
WORKDIR /app
COPY go.mod go.sum Makefile ./
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
# CA bundle for the Go binaries' HTTPS calls, copied from the build image instead of apt
# (headless-shell 114 is Debian bullseye; installing packages there is fragile).
COPY --from=build-stage /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
WORKDIR /
COPY --from=client-build-stage /client /client
COPY --from=build-stage /app/internal /internal
EXPOSE 8080
ENTRYPOINT ["/client"]

# Worker release stage
FROM chromedp/headless-shell:114.0.5735.199 AS worker-release-stage
# CA bundle for the Go binaries' HTTPS calls, copied from the build image instead of apt
# (headless-shell 114 is Debian bullseye; installing packages there is fragile).
COPY --from=build-stage /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
WORKDIR /
COPY --from=worker-build-stage /worker /worker
COPY --from=build-stage /app/internal /internal
EXPOSE 8081
ENTRYPOINT ["/worker"]