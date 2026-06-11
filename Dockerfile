# ビルド: 静的シングルバイナリ(CGO 不要・tzdata 埋め込み済み)
FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bot ./cmd/bot

# 実行: distroless でイメージ最小化
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bot /bot
ENV EAB_DB_PATH=/data/eab.db
VOLUME /data
ENTRYPOINT ["/bot"]
