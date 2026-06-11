# eorzea-activity-board 開発用 Makefile
# 起動には .env(gitignore 済み)に DISCORD_TOKEN=... が必要

.PHONY: build vet test check run docker clean

build: ## ビルド
	go build ./...

vet: ## 静的チェック
	go vet ./...

test: ## ユニットテスト(store 層 / bot ロジック)
	go test ./...

check: build vet test ## ビルド + vet + テストをまとめて実行

run: ## .env を読み込んで Bot を起動(TZ は内部で Asia/Tokyo 固定)
	@test -f .env || { echo "❌ .env がありません。DISCORD_TOKEN=... を書いた .env を作成してください"; exit 1; }
	set -a; . ./.env; set +a; go run ./cmd/bot

docker: ## Docker イメージをビルド
	docker build -t eorzea-activity-board .

clean: ## ビルドキャッシュとテストキャッシュを削除
	go clean -cache -testcache

help: ## ターゲット一覧
	@grep -E '^[a-z]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-8s %s\n", $$1, $$2}'
