# Go dev image for the worker and api.
#
# Source is bind-mounted at /app (see docker-compose.yml) and the service
# runs via `go run`, so no compiled binary is baked into the image and one image
# serves both commands. File-watch hot reload is deliberately NOT used: inotify
# events don't reliably cross the host→VM boundary for bind mounts on Docker
# Desktop, so a watcher never fires. To pick up code changes, recompile by
# restarting the container: `make restart SVC=api` (or `SVC=worker`).
#
# The Go module and build caches live on named volumes mounted by compose, so
# they survive restarts and are shared across git worktrees — the first
# `go run` is cold, every one after is seconds.
FROM golang:1.26-alpine

# git: the go toolchain uses it for some module fetches. ca-certificates: TLS.
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Pure-Go build (the imaging deps are cgo-free); alpine has no C toolchain.
ENV CGO_ENABLED=0

EXPOSE 8080
