FROM --platform=$BUILDPLATFORM node:16 AS builder

# 构建环境说明：此前 a571eab 曾把前端构建镜像升级到 node 24，但实测 node 24 的 npm 会让
# 三个主题的 react-scripts build 全部失败；而前端 package.json 的依赖自那次升级以来
# 从未改动（git diff a571eab HEAD -- web/*/package.json 为空），说明问题来自构建环境而非
# 依赖本身。node 16 是当初 v0.0.1-fix-blank-20260902 能正常产出前端的组合，故回退到它。
# 注意：node 16 使用 OpenSSL 1.1，不要再加 ENV NODE_OPTIONS=--openssl-legacy-provider，
# 该选项是 node 17+ 才有、node 16 无法识别。

WORKDIR /web
COPY ./VERSION .
COPY ./web .

RUN npm install --legacy-peer-deps --no-audit --no-fund --fetch-retries=5 --fetch-retry-mintimeout=10000 --fetch-retry-maxtimeout=60000 --prefix /web/default && \
    npm install --legacy-peer-deps --no-audit --no-fund --fetch-retries=5 --fetch-retry-mintimeout=10000 --fetch-retry-maxtimeout=60000 --prefix /web/berry && \
    npm install --legacy-peer-deps --no-audit --no-fund --fetch-retries=5 --fetch-retry-mintimeout=10000 --fetch-retry-maxtimeout=60000 --prefix /web/air

# 串行构建三个主题。任一主题失败立即中断，让 npm 的原始报错出现在构建日志里。
# 此前用并行 & + wait 会吞掉单个主题的失败退出码，导致没有产物却"构建成功"。
# 各主题 package.json 的 build 脚本末尾有 mv，但这里用 --prefix 调用 npm run，
# 工作目录仍是 WORKDIR /web，脚本里的 ../build 指向不存在的 /build，所以 mv 不会生效，
# 产物始终留在各主题自己的 /web/<theme>/build —— 下方 COPY 的源也必须用这个路径。
RUN DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/default && \
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/berry && \
    DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ./VERSION) npm run build --prefix /web/air

# 严格校验：产物必须存在，且 index.html 必须带打包生成的 script 标签。
# 不能拿混在主题目录里的源模板 public/index.html 顶替（它永远存在但没有 bundle，
# 嵌进去就是没有 JS 的空壳页面 → 白屏）。
RUN set -e; \
    for t in default berry air; do \
      p=/web/$t/build/index.html; \
      if [ ! -s "$p" ]; then echo "ERROR: missing frontend build for theme $t at $p"; exit 1; fi; \
      if ! grep -q '<script' "$p"; then \
        echo "ERROR: theme $t index.html has no bundle script"; cat "$p"; exit 1; \
      fi; \
      echo "theme $t OK -> $p"; \
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
# 产物在各主题自己的 build 目录（mv 不生效，原因见上方注释）。必须按主题逐个复制：
# 若整块复制 /web/build，那里只有被 git 跟踪的占位 .gitkeep，嵌进去的前端为空就会白屏。
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
