FROM --platform=$BUILDPLATFORM node:24 AS builder

# Node 17+ 默认 OpenSSL 3，react-scripts(webpack) 需要 legacy provider 才能构建
ENV NODE_OPTIONS=--openssl-legacy-provider

WORKDIR /web
COPY ./VERSION .
COPY ./web .

RUN npm install --legacy-peer-deps --prefix /web/default & \
    npm install --legacy-peer-deps --prefix /web/berry & \
    npm install --legacy-peer-deps --prefix /web/air & \
    wait

RUN DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/default & \
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/berry & \
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/air & \
    wait

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
# 前端由 npm run build 直接产出到 builder 阶段的 /web/<theme>/build（CRA 默认输出目录），
# 这里显式按主题拷贝进 web/build/<theme>，再被 //go:embed web/build/* 嵌入（否则白屏）。
# 注意：不要再依赖 package.json 里的 mv 汇总，已被移除；此处的显式 COPY 更可靠且能绕过 gha 缓存。
COPY --from=builder /web/default/build ./web/build/default
COPY --from=builder /web/berry/build ./web/build/berry
COPY --from=builder /web/air/build ./web/build/air

RUN go build -trimpath -ldflags "-s -w -X 'github.com/songquanpeng/one-api/common.Version=$(cat VERSION)' -linkmode external -extldflags '-static'" -o one-api

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder2 /build/one-api /

EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/one-api"]