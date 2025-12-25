# 阶段 1: 构建
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY main.go .
# 禁用 CGO 以确保静态链接
RUN CGO_ENABLED=0 go build -o tcp-echo main.go

# 阶段 2: 运行
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/tcp-echo .
EXPOSE 3333
CMD ["./tcp-echo"]