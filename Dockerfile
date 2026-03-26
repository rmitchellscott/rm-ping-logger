FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.9.0 AS xx

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
WORKDIR /app
COPY --from=xx / /
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
ARG TARGETPLATFORM

RUN --mount=type=cache,target=/root/.cache \
    CGO_ENABLED=0 xx-go build \
    -ldflags="-w -s" \
    -trimpath \
    -o rm-ping-logger

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && update-ca-certificates
COPY --from=builder /app/rm-ping-logger /usr/local/bin/
ENV PORT=8080
ENTRYPOINT ["/usr/local/bin/rm-ping-logger"]
