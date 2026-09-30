# syntax=docker/dockerfile:1
# Build context is apps/dashboard-api (this Go module is self-contained).

FROM golang:1.25-alpine AS build
WORKDIR /src
# Cache deps first.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Static binary so the runtime image needs no libc.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/dashboard-api ./cmd/server

FROM alpine:3.20 AS runtime
# ca-certificates: needed for TLS to the k3s API server.
# tmux: client for the host's tmux server (Claude session). Must speak the same
# protocol as the host's tmux (3.4 on both today).
RUN apk add --no-cache ca-certificates tmux && adduser -D -u 10001 app
COPY --from=build /out/dashboard-api /usr/local/bin/dashboard-api
USER app
ENV PORT=3001
EXPOSE 3001
ENTRYPOINT ["/usr/local/bin/dashboard-api"]
