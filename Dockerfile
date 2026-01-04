# 阶段 1: 构建 Go 应用
FROM golang:1.24-alpine AS builder

# 设置工作目录
WORKDIR /app

# 复制 go.mod 和 go.sum 文件并下载依赖
# 这样可以利用 Docker 的层缓存机制
COPY go.mod go.sum ./
RUN go mod download

# 复制所有源代码
COPY . .

# 编译 Go 应用
# CGO_ENABLED=0 创建一个静态链接的可执行文件，使其可以在一个干净的 alpine 镜像中运行
# -o /app/main 指定输出文件
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/main .

# 阶段 2: 创建最终的轻量级镜像
FROM alpine:latest

WORKDIR /app

# 从 builder 阶段复制编译好的二进制文件
COPY --from=builder /app/main .

# 暴露的端口 (如果您的应用有 http 服务)
# EXPOSE 8080

# 容器启动时运行的命令
CMD ["/app/main"]

