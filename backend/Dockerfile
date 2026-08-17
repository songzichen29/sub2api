FROM golang:1.26.6-alpine

WORKDIR /app

# 安装必要的工具（tzdata 提供 zoneinfo，配合 ENV TZ 让容器时区与宿主机一致）
RUN apk add --no-cache git tzdata

# 复制go.mod和go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN VERSION_VALUE="$(./scripts/resolve-version.sh)" && \
    go build -ldflags="-s -w -X main.Version=${VERSION_VALUE}" -o main ./cmd/server/

# 暴露端口
EXPOSE 8080

# 容器时区与宿主机（America/Los_Angeles）一致；可被 compose 的 TZ 环境变量覆盖
ENV TZ=America/Los_Angeles

# 运行应用
CMD ["./main"]
