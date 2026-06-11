# eorzea-activity-board

身内の FF14 Discord グループ向け、日々の活動(レベルレ・モブハン・PW作成・地図)を
ゆるくマッチングする低コスト Bot。Go + discordgo + SQLite(modernc, CGO 不要)の
1 プロセス構成。要件は `docs/requirements.md`、設計は `docs/design.md` を参照。

## 開発の前提

- 開発はこの Devcontainer 内で行う(Go 1.24 / Claude Code / sqlite3 導入済み)
- ユーザー操作はボタン・セレクト・モーダルのみ。Slash Command は使わない
- 管理者だけ `!eab setup` / `!eab post` のテキストコマンドを使う
- 低コスト・低運用負荷・シンプルさ最優先。マルチギルド対応や高機能化はしない

## よく使うコマンド

```bash
go mod tidy          # 依存解決(初回必須: go.sum 未生成)
go build ./...       # ビルド
go vet ./...         # 静的チェック
DISCORD_TOKEN=... go run ./cmd/bot   # 起動(TZ は内部で Asia/Tokyo 固定)
```

## 現状

- MVP 実装一式 + ユニットテスト(store 層 / candidatesFor / progressBar など)済み
- **実 Discord サーバーで全フロー動作確認済み**(2026-06-11):
  `!eab setup` → 設定セレクト3種 → 今日の状態 → PW モーダル →
  `!eab post`(サマリー)→ 活動選択 → 募集カード → 参加 → スレッド自動作成
- 20 時投稿は**サマリー型**: サマリー 1 通だけ流し、セレクトで選ばれた活動の
  募集カードを遅延生成する(ノイズ抑制)。詳細は `docs/design.md`
- 開発時の起動: `.env`(gitignore 済み)に `DISCORD_TOKEN=...` を書き、
  `set -a; . ./.env; set +a; go run ./cmd/bot`

## 設計上の約束ごと

- CustomID はステートレス(`prop:join:<proposalID>` 形式)。再起動後も過去メッセージの
  ボタンが機能する状態を維持すること
- 日付はすべて JST の `YYYY-MM-DD`。`bot.today()` を使う
- アクティビティ・時間帯・スタンスの追加変更は `internal/activity/activity.go` のみ
- SQLite は `SetMaxOpenConns(1)` + WAL。スキーマ変更は `internal/store/store.go` の
  `schema` 定数に追記(`CREATE TABLE IF NOT EXISTS` ベースの素朴な管理)
