FROM golang:1.24-alpine AS builder

WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/pet-ring ./cmd/pet-ring

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 pet-ring \
    && adduser -S -D -H -u 10001 -G pet-ring pet-ring \
    && mkdir -p /data \
    && chown 10001:10001 /data

COPY --from=builder /out/pet-ring /usr/local/bin/pet-ring
USER 10001:10001
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/pet-ring"]
