FROM --platform=$BUILDPLATFORM node:24 AS builder

# Node 17+ 默认 OpenSSL 3，react-scripts(webpack) 需要 legacy provider 才能构建
ENV NODE_OPTIONS=--openssl-legacy-provider

WORKDIR /web
COPY ./VERSION .
COPY ./web .

RUN npm install --legacy-peer-deps --no-audit --no-fund --fetch-retries=5 --fetch-retry-mintimeout=10000 --fetch-retry-maxtimeout=60000 --prefix /web/default && \
    npm install --legacy-peer-deps --no-audit --no-fund --fetch-retries=5 --fetch-retry-mintimeout=10000 --fetch-retry-maxtimeout=60000 --prefix /web/berry && \
    npm install --legacy-peer-deps --no-audit --no-fund --fetch-retries=5 --fetch-retry-mintimeout=10000 --fetch-retry-maxtimeout=60000 --prefix /web/air

# 串行执行：此前并行 + wait 会吞掉单个主题的失败退出码，导致某主题构建失败时产物
# 缺失、镜像却照样构建成功，最终该主题白屏。
# 注意 build 脚本末尾的 "mv -f build ../build/<theme>" 实际不生效：这里用 --prefix
# 调用 npm run，工作目录仍是 WORKDIR /web，脚本里的 ../build 指向 /build（不存在），
# 所以产物始终留在各主题自己的 /web/<theme>/build 内 —— 下方 COPY 源也必须用这个路径。
RUN DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/default && \
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/berry && \
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/air && \
    test -s /web/default/build/index.html && \
    test -s /web/berry/build/index.html && \
    test -s /web/air/build/index.html

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
# 产物实际留在各主题的 /web/<theme>/build（mv 不生效，原因见上方注释）。
# 必须按主题逐个复制：若整块复制 /web/build，那里只有被 git 跟踪的占位 .gitkeep，
# 嵌进二进制的前端为空就会导致白屏。逐个复制可让任一产物缺失立刻报错，
# 而不是"构建绿却白屏"。
COPY --from=builder /web/default/build ./web/build/default
COPY --from=builder /web/berry/build ./web/build/berry
COPY --from=builder /web/air/build ./web/build/air

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
