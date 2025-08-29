# 多阶段构建 Dockerfile
# 第一阶段：构建阶段
FROM golang:1.21-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装必要的工具
RUN apk add --no-cache git ca-certificates tzdata

# 复制 go mod 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用程序
# 使用 CGO_ENABLED=0 来构建静态链接的二进制文件
# 添加版本信息
ARG VERSION=dev
ARG BUILD_TIME
ARG GIT_COMMIT

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}" \
    -o alicloud-exporter \
    cmd/alicloud-exporter/main.go

# 第二阶段：运行阶段
FROM alpine:3.18

# 安装 ca-certificates 和 tzdata
RUN apk --no-cache add ca-certificates tzdata

# 创建非 root 用户
RUN addgroup -g 1001 -S exporter && \
    adduser -u 1001 -S exporter -G exporter

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/alicloud-exporter /usr/local/bin/alicloud-exporter

# 复制配置文件模板
COPY --from=builder /app/config/config.yaml /app/config/config.yaml

# 创建必要的目录并设置权限
RUN mkdir -p /app/config /app/logs && \
    chown -R exporter:exporter /app && \
    chmod +x /usr/local/bin/alicloud-exporter

# 切换到非 root 用户
USER exporter

# 暴露端口
EXPOSE 9100

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9100/health || exit 1

# 设置环境变量
ENV ALICLOUD_EXPORTER_CONFIG_FILE=/app/config/config.yaml
ENV ALICLOUD_EXPORTER_LOG_LEVEL=info
ENV ALICLOUD_EXPORTER_LOG_FORMAT=json

# 启动命令
CMD ["alicloud-exporter", "--config", "/app/config/config.yaml"]

# 添加标签
LABEL maintainer="alicloud-exporter" \
      description="Prometheus exporter for Alibaba Cloud services" \
      version="${VERSION}" \
      org.opencontainers.image.title="alicloud-exporter" \
      org.opencontainers.image.description="Prometheus exporter for Alibaba Cloud services" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.source="https://github.com/your-org/alicloud-exporter" \
      org.opencontainers.image.licenses="MIT"