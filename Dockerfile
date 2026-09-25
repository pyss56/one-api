FROM --platform=$BUILDPLATFORM node:24 AS builder

# Node 17+ 默认 OpenSSL 3，react-scripts(webpack) 需要 legacy provider 才能构建
ENV NODE_OPTIONS=--openssl-legacy-provider

WORKDIR /web
COPY ./VERSION .
COPY ./web .

RUN npm install --legacy-peer-deps --no-audit --no-fund --prefix /web/default && \
    npm install --legacy-peer-deps --no-audit --no-fund --prefix /web/berry && \
    npm install --legacy-peer-deps --no-audit --no-fund --prefix /web/air

# default 主题必选（构建失败则整体失败）；berry/air 可选，构建失败仅跳过该主题
RUN DISABLE_ESLINT_PLUGIN='true' CI='false' TSC_COMPILE_ON_ERROR='true' \
      REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/default
RUN for t in berry air; do \
      DISABLE_ESLINT_PLUGIN='true' CI='false' TSC_COMPILE_ON_ERROR='true' \
      REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/$t \
      || echo "WARN: theme '$t' build failed, skipping"; \
    done

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
# 各主题 build 脚本：react-scripts build && mv -f build ../build/<theme>
# 产物汇总到 builder 阶段的 /web/build/{default,berry,air}，整体拷入后被 //go:embed web/build/* 嵌入。
COPY --from=builder /web/build ./web/build

RUN go build -trimpath -ldflags "-s -w -X 'github.com/songquanpeng/one-api/common.Version=$(cat VERSION)' -linkmode external -extldflags '-static'" -o one-api

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder2 /build/one-api /

EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/one-api"]