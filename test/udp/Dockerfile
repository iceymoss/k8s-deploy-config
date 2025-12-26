FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY main.go .
RUN CGO_ENABLED=0 go build -o udp-server main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/udp-server .
# ⚠️ 注意这里标明 UDP
EXPOSE 4444/udp
CMD ["./udp-server"]