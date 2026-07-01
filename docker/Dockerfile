# syntax=docker/dockerfile:1

# Generic build for any Torque backend entrypoint under ./cmd.
# Select the target with build-arg CMD (e.g. api, worker).
ARG GO_VERSION=1.26.4

FROM golang:${GO_VERSION}-alpine AS builder

ARG CMD=api

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/app ./cmd/${CMD}

FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
    addgroup -S app && adduser -S -G app app

WORKDIR /app

COPY --from=builder /out/app /app/app
# OpenAPI spec is embedded via docs/docs.go; nothing else required at runtime.

USER app

ENTRYPOINT ["/app/app"]
