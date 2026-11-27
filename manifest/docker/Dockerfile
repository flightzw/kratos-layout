# 使用 make docker 构建，Go 版本与 go.mod 保持一致。
ARG GO_VERSION=1.26.8
FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

# 先下载依赖，复用构建缓存。
ARG GOPROXY=https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 固定产物名，兼容模板入口改名；多个入口时指定 APP_PATH。
ARG APP_PATH=./cmd/...
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o /out/server ${APP_PATH}

FROM alpine:3.24

# 静态程序仅需 HTTPS 根证书。
RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /out/server ./server

EXPOSE 8000
EXPOSE 9000
# 挂载配置目录，通过 KRATOS_DATABASE_SOURCE 注入数据库连接。
VOLUME /data/conf

CMD ["./server", "-conf", "/data/conf"]
