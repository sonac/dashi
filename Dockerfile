FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk --no-cache add gcc musl-dev
COPY go.mod ./
COPY go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -o /out/dashi ./cmd/server

FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata sqlite
WORKDIR /app
COPY --from=build /out/dashi /usr/local/bin/dashi
EXPOSE 8080
ENV APP_DATA_DIR=/data
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/dashi"]
