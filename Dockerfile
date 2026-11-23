# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

ENV GOTOOLCHAIN=auto \
    CGO_ENABLED=0

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w \
      -X github.com/PapaDanielVi/cafebazaar_cli/internal/cli.Version=${VERSION} \
      -X github.com/PapaDanielVi/cafebazaar_cli/internal/cli.Commit=${COMMIT} \
      -X github.com/PapaDanielVi/cafebazaar_cli/internal/cli.BuildDate=${BUILD_DATE}" \
    -o /out/bazaar ./cmd/bazaar

FROM alpine:3.21

LABEL org.opencontainers.image.title="bazaar" \
      org.opencontainers.image.description="CafeBazaar CLI for Android app publishing, Pishkhan releases, AAB/APK uploads, and user reviews" \
      org.opencontainers.image.url="https://github.com/PapaDanielVi/cafebazaar_cli" \
      org.opencontainers.image.source="https://github.com/PapaDanielVi/cafebazaar_cli" \
      org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache ca-certificates git tzdata \
    && adduser -D -u 10001 bazaar

COPY --from=builder /out/bazaar /usr/local/bin/bazaar

ENV BAZAAR_NO_KEYRING=1 \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8

WORKDIR /workspace
USER bazaar

ENTRYPOINT ["bazaar"]
CMD ["--help"]
