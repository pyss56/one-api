FROM --platform=$BUILDPLATFORM node:20 AS builder

# 构建环境说明：
# 1) 必须用 yarn 而不是 npm。npm 的 hoisting 会把这条依赖链解成互斥组合 ——
#    顶层被 ajv@6 占据，而 ajv-keywords@5 的 peer 要 ajv@8，
#    于是 react-scripts build 报 MODULE_NOT_FOUND: ajv/dist/compile/codegen。
#    实测 node16/npm8、node20/npm10、node24 都会解崩，只有 yarn 能给出可用组合。
#    已提交 yarn.lock 固定版本，避免上游再次漂移。
# 2) node 版本必须 >=18：node-releases 等依赖已要求 node >= 18，node 16 会直接被拒装。
WORKDIR /web
COPY ./VERSION .
COPY ./web .

# yarn 按工作目录安装，所以逐个 cd 进主题目录
RUN cd /web/default && yarn install --non-interactive && \
    cd /web/berry && yarn install --non-interactive && \
    cd /web/air && yarn install --non-interactive

# 逐个主题构建，任一失败立即中断，让原始报错出现在构建日志里
# （此前并行 + wait 会吞掉单个主题的失败退出码，导致没有产物却"构建成功"）
RUN cd /web/default && DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ../VERSION) yarn build && \
    cd /web/berry && DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ../VERSION) yarn build && \
    cd /web/air && DISABLE_ESLINT_PLUGIN='true' REACT_APP_VERSION=$(cat ../VERSION) yarn build

# 统一汇总产物到 /web/artifacts/<theme>。
# 不能只按路径猜：各主题 build 脚本末尾的 "mv -f build ../build/<theme>" 是否生效
# 取决于工作目录，实测两种都出现过（留在 /web/<theme>/build 或被移到 /web/build/<theme>），
# 所以两个候选位置都检查；同时强制校验 index.html 带有打包生成的 script 标签，
# 避免把未打包的 public/index.html 源模板当成产物（那会嵌出没有 JS 的白屏空壳）。
RUN set -e; \
    for t in default berry air; do \
      src=""; \
      for p in /web/$t/build/index.html /web/build/$t/index.html; do \
        if [ -s "$p" ]; then src=$(dirname "$p"); break; fi; \
      done; \
      if [ -z "$src" ]; then echo "ERROR: no build output for theme $t"; exit 1; fi; \
      if ! grep -q '<script' "$src/index.html"; then \
        echo "ERROR: theme $t index.html has no bundle script"; cat "$src/index.html"; exit 1; \
      fi; \
      echo "theme $t artifacts -> $src"; \
      mkdir -p /web/artifacts/$t; \
      cp -a "$src/." /web/artifacts/$t/; \
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
# 产物已由 builder 阶段统一汇总到 /web/artifacts/<theme>。
# 必须按主题逐个复制：若整块复制 /web/build，那里只有被 git 跟踪的占位 .gitkeep，
# 嵌进二进制的前端为空就会导致白屏。
COPY --from=builder /web/artifacts/default ./web/build/default
COPY --from=builder /web/artifacts/berry ./web/build/berry
COPY --from=builder /web/artifacts/air ./web/build/air

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
