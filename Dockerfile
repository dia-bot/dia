# Builds the Dia Go services (worker + api) as static binaries.
# The same image runs either binary — the compose file picks the command.
FROM golang:1.26-alpine AS build
RUN apk add --no-cache git
WORKDIR /src

# Dependency layer (cached). The vendored discordgo lives in-module under
# pkg/discordgo, so only external modules are downloaded here.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 dia
WORKDIR /app
COPY --from=build /out/worker /out/api /app/
COPY assets/ /app/assets/
USER dia
EXPOSE 8080
# Override in compose: ["/app/worker"] or ["/app/api"].
CMD ["/app/api"]
