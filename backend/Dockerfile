FROM golang:1.27.2-alpine

WORKDIR /app

# 安装必要的工具（tzdata 提供 zoneinfo，配合 ENV TZ 让容器时区与宿主机一致）
RUN apk add --no-cache git tzdata font-wqy-zenhei
RUN apk add --no-cache --virtual .qqbot-font-build py3-fonttools \
    && python3 -c "from fontTools.ttLib import TTFont; TTFont('/usr/share/fonts/wqy-zenhei/wqy-zenhei.ttc', fontNumber=0).save('/usr/share/fonts/wqy-zenhei/wqy-zenhei.ttf')" \
    && apk del .qqbot-font-build

# 复制go.mod和go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN VERSION_VALUE="$(./scripts/resolve-version.sh)" && \
    go build -ldflags="-s -w -X main.Version=${VERSION_VALUE}" -o main ./cmd/server/ \
    && ./main --qqbot-card-check

# 暴露端口
EXPOSE 8080

# 默认使用上海时区；可被运行时 TZ 环境变量覆盖
ENV TZ=Asia/Shanghai

# 运行应用
CMD ["./main"]
