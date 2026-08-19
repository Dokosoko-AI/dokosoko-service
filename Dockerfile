FROM node:22-alpine AS console-builder
WORKDIR /src
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY app ./app
COPY public ./public
COPY worker ./worker
COPY .openai ./.openai
COPY next.config.ts postcss.config.mjs tsconfig.json vite.config.ts next-env.d.ts ./
RUN pnpm build

FROM golang:1.25-alpine AS service-builder
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/dokosoko ./cmd/dokosoko

FROM alpine:3.22
RUN addgroup -S -g 65532 dokosoko && adduser -S -D -H -u 65532 -G dokosoko dokosoko
WORKDIR /app
COPY --from=service-builder /out/dokosoko /app/dokosoko
COPY --from=console-builder /src/dist/client /app/ui
COPY migrations /app/migrations
RUN mkdir -p /data && chown -R dokosoko:dokosoko /data /app
USER dokosoko
ENV DOKOSOKO_LISTEN=:8080 DOKOSOKO_UI_DIR=/app/ui DOKOSOKO_DATA_DIR=/data DOKOSOKO_MIGRATIONS_DIR=/app/migrations
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/dokosoko"]
