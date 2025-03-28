FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o shard ./cmd/shard/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/shard /app/shard

RUN mkdir -p /app/out
CMD ["/app/shard"]
