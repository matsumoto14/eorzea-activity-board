# eorzea-activity-board

身内の FF14 Discord グループ向け、日々の活動(レベルレ・モブハン・PW作成・地図)を
ゆるくマッチングする低コスト Bot。Go + discordgo + SQLite(modernc, CGO 不要)の
1 プロセス構成。要件は `docs/requirements.md`、設計は `docs/design.md` を参照。

## 開発の前提

- 開発はこの Devcontainer 内で行う(Go 1.24 / Claude Code 導入済み)
- ユーザー操作はボタン・セレクト・モーダルのみ。Slash Command は使わない
- 管理者だけ `!eab setup` / `!eab post` のテキストコマンドを使う
- 低コスト・低運用負荷・シンプルさ最優先。マルチギルド対応や高機能化はしない

## よく使うコマンド

```bash
go build ./...       # ビルド
go vet ./...         # 静的チェック
go test ./...        # ユニットテスト(store 層 / bot ロジック)

# 起動: .env(gitignore 済み)に DISCORD_TOKEN=... を書いておく
set -a; . ./.env; set +a; go run ./cmd/bot   # TZ は内部で Asia/Tokyo 固定
```

- この Devcontainer は `/go` が root 所有のため、モジュールキャッシュは
  `go env -w GOMODCACHE=$HOME/go/pkg/mod GOPATH=$HOME/go` 設定済み
- `sqlite3` CLI は無い。DB の中身を見るときは Go の小ツールを書くか
  `modernc.org/sqlite` 経由で読む

## 現状(2026-06-11 時点)

- MVP 実装一式 + ユニットテストを初回コミット済み(`20be794`)
- **実 Discord サーバーで全フロー動作確認済み**:
  `!eab setup` → 設定セレクト3種 → 今日の状態 → PW モーダル →
  `!eab post`(サマリー)→ 活動選択 → 募集カード → 参加 → スレッド自動作成
- 20 時投稿は**サマリー型**: サマリー 1 通だけ流し、セレクトで選ばれた活動の
  募集カードを遅延生成する(ノイズ抑制)。詳細は `docs/design.md`
- code-review で堅牢化済み: 3秒応答期限(deferred ack)、提案操作の直列化、
  DB/Discord 不整合の自己修復、embed 文字数キャップ、サマリーの同日重複防止
- 未実施: 本番ホスティングへのデプロイ(Dockerfile は用意済み)

## 設計上の約束ごと

- CustomID はステートレス(`prop:join:<proposalID>` / `summary:open:<日付>` 形式)。
  再起動後も過去メッセージのボタンが機能する状態を維持すること
- 日付はすべて JST の `YYYY-MM-DD`。`bot.today()` を使う
- アクティビティ・時間帯・スタンスの追加変更は `internal/activity/activity.go` のみ
- SQLite は `SetMaxOpenConns(1)` + WAL。スキーマ変更は `internal/store/store.go` の
  `schema` 定数に追記(`CREATE TABLE IF NOT EXISTS` ベースの素朴な管理)

## ハンドラ実装の約束ごと(堅牢性)

- 重い処理(Discord API 呼び出し等)を含むハンドラは**先に deferred ack** してから
  処理し、結果は `InteractionResponseEdit` で反映する(3 秒期限対策)
- 提案の作成・成立判定は `Bot.propMu` で直列化する(同時押しの二重作成防止)
- DB と Discord の不整合は押下時に自己修復する設計を崩さない:
  `message_id` 欠落はボタンの付いたメッセージ自身から復元、`thread_id` 欠落は
  既存スレッド(メッセージ起点スレッドの ID = 元メッセージ ID)を拾い直す
- embed に入れるユーザー由来の文字列は `limitEmbed`(field 1024 / description 4096)
  でキャップする
- サマリーの同日重複は settings の `summary_date` キーで防止(`!eab post` は
  スキップ時にその旨を返信する)
