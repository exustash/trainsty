# Containerfile — build a static trainsty binary and package it in a minimal image.
#
# trainsty adheres to Principle I: zero external dependencies, Go standard library only.
# The binary is statically compiled (CGO_ENABLED=0) and uses the foreground 'serve'
# subcommand as the entrypoint (since 'start' forks with setsid and exits 0, which would
# terminate PID 1 in a container).

FROM docker.io/library/golang:1.27.3-alpine3.24 AS builder

WORKDIR /src

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/trainsty .

FROM docker.io/library/alpine:3.24

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /out/trainsty /usr/local/bin/trainsty

# trainsty listens on the hardcoded port 45678 (ADR-011)
EXPOSE 45678

# By default, run the scheduler in the foreground.
# Setting ENTRYPOINT to trainsty allows passing any subcommand (e.g. status, wrap, version).
ENTRYPOINT ["/usr/local/bin/trainsty"]
CMD ["serve"]
