FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/backend-integration ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app
COPY --from=build /out/backend-integration /usr/local/bin/backend-integration

USER app
EXPOSE 8081
HEALTHCHECK --interval=10s --timeout=2s --start-period=5s --retries=5 \
    CMD wget -q -O /dev/null http://127.0.0.1:8081/readyz || exit 1
ENTRYPOINT ["/usr/local/bin/backend-integration"]
