FROM --platform=$BUILDPLATFORM node:24 AS builder

# Node 17+ 默认 OpenSSL 3，react-scripts(webpack) 需要 legacy provider 才能构建
ENV NODE_OPTIONS=--openssl-legacy-provider

WORKDIR /web
COPY ./VERSION .
COPY ./web .

RUN npm install --legacy-peer-deps --no-audit --no-fund --fetch-retries=5 --fetch-retry-mintimeout=10000 --fetch-retry-maxtimeout=60000 --prefix /web/default && \
    npm install --legacy-peer-deps --no-audit --no-fund --fetch-retries=5 --fetch-retry-mintimeout=10000 --fetch-retry-maxtimeout=60000 --prefix /web/berry && \
    npm install --legacy-peer-deps --no-audit --no-fund --fetch-retries=5 --fetch-retry-mintimeout=10000 --fetch-retry-maxtimeout=60000 --prefix /web/air

# 各主题 build 脚本会把产物 mv 到 ../build/<theme>，先确保该目录存在。
# 串行执行：此前并行 + wait 会吞掉单个主题的失败退出码，导致某个主题（default）
# 构建失败时产物缺失、镜像却照样构建成功，最终前端白屏。
RUN mkdir -p /web/build && \
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/default && \
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/berry && \
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/air && \
    test -s /web/build/default/index.html && \
    test -s /web/build/berry/index.html && \
    test -s /web/build/air/index.html

FROM golang:alpine AS builder2

RUN apk add --no-cache \
    gcc \
    musl-dev \
    sqlite-dev \
    build-base

ENV GO111MODULE=on \
    CGO_ENABLED=1 \
    GOOS=linux

WORKDIR /build

ADD go.mod go.sum ./
RUN go mod download

COPY . .
# 前端 build 脚本会把产物 mv 到 builder 阶段的 /web/build/{default,berry,air}
# （见各主题 package.json：react-scripts build && mv -f build ../build/<theme>）。
# 必须按主题逐个复制：整块复制 /web/build 时，若任一主题构建失败（其目录不存在）
# 构建仍会成功，缺的那个主题就会白屏。逐个复制可让缺失立刻报错。
COPY --from=builder /web/build/default ./web/build/default
COPY --from=builder /web/build/berry ./web/build/berry
COPY --from=builder /web/build/air ./web/build/air

# 兜底校验：//go:embed web/build/* 要求这三个目录内确实有产物
RUN test -s web/build/default/index.html && \
    test -s web/build/berry/index.html && \
    test -s web/build/air/index.html

RUN go build -trimpath -ldflags "-s -w -X 'github.com/songquanpeng/one-api/common.Version=$(cat VERSION)' -linkmode external -extldflags '-static'" -o one-api

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder2 /build/one-api /

EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/one-api"]
