# syntax=docker/dockerfile:1

FROM golang:1.26.5-alpine AS build

WORKDIR /src

RUN apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/backup ./cmd/backup

FROM alpine:3.22 AS runtime

RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY configs ./configs
# Named-volume mount point: must exist and be writable by the app user,
# otherwise file uploads fail with permission denied after the first boot.
RUN mkdir -p /var/lib/server/uploads && chown -R app:app /var/lib/server
USER app

FROM runtime AS api

COPY --from=build /out/api ./api
EXPOSE 8080
ENTRYPOINT ["/app/api"]

FROM runtime AS migrate

COPY --from=build /out/migrate ./migrate
COPY migrations ./migrations
ENTRYPOINT ["/app/migrate"]

FROM runtime AS backup

COPY --from=build /out/backup ./backup
ENTRYPOINT ["/app/backup"]
