# eorzea-activity-board

身内の FF14 Discord グループ向け、日々の活動(ルーレット・モブハン・極・零式・地図など
17 カテゴリ + 種目)をゆるくマッチングする低コスト Bot。Go + discordgo + SQLite(modernc, CGO 不要)の
1 プロセス構成。要件は `docs/requirements.md`、設計は `docs/design.md` を参照。

## 開発の前提

- 開発はこの Devcontainer 内で行う(Go 1.24 / Claude Code 導入済み)
- ユーザー操作はボタン・セレクト・モーダルのみ。Slash Command は使わない
- 管理者だけ `!eab setup` / `!eab post` のテキストコマンドを使う
- 低コスト・低運用負荷・シンプルさ最優先。マルチギルド対応や高機能化はしない

## よく使うコマンド

```bash
make check           # build + vet + test をまとめて実行(コミット前はこれ)
make run             # .env を読み込んで Bot を起動(TZ は内部で Asia/Tokyo 固定)
make help            # ターゲット一覧(docker / clean など)
```

- 起動には `.env`(gitignore 済み)に `DISCORD_TOKEN=...` を書いておく
- `make` を使わない場合は `go build ./...` / `go vet ./...` / `go test ./...` と
  `set -a; . ./.env; set +a; go run ./cmd/bot` がそのまま使える

- `/go/pkg` はモジュールキャッシュ永続化用の名前付きボリューム(`eab-go-cache`)。
  初回マウント時に root 所有になる問題は `post-create.sh` の chown で対処済み。
  もし gopls が `mkdir /go/pkg/mod: permission denied` で落ちたら
  `sudo chown -R vscode:golang /go/pkg` を実行する(GOMODCACHE はデフォルトのまま使う)
- DB の中身は `sqlite3 data/eab.db`(CLI)で確認できる(post-create.sh で導入)

## 現状(2026-06-11 時点)

- MVP 実装一式 + ユニットテストを初回コミット済み(`20be794`)
- **実 Discord サーバーで全フロー動作確認済み**:
  `!eab setup` → 設定セレクト3種 → 今日の状態 → PW モーダル →
  `!eab post`(サマリー)→ 活動選択 → 募集カード → 参加 → スレッド自動作成
- 20 時投稿は**サマリー型**: サマリー 1 通だけ流し、セレクトで選ばれた活動の
  募集カードを遅延生成する(ノイズ抑制)。詳細は `docs/design.md`
- code-review で堅牢化済み: 3秒応答期限(deferred ack)、提案操作の直列化、
  DB/Discord 不整合の自己修復、embed 文字数キャップ、サマリーの同日重複防止
- UX 改善済み: embed は余白重視のレイアウト(inline は 2 列まで・一覧は 1 人 2 行)、
  文言はヒルディブランド調
- アクティビティは **2 階層(カテゴリ 17 + 種目)**:
  マッチング(設定・サマリー・候補人数)はカテゴリ単位、種目はサマリーで活動を
  選んだときにエフェメラルのセレクトで 1 回だけ聞く(「相談して決める」も可)。
  種目はカードのタイトル・スレッド名に「地図(G17)」形式で載る。
  旧 ID(levelre / pw など)は `activity.NormalizeIDs` / `ByID` が読み出し時に吸収し、
  メンバーの再登録は不要。PW 進捗は「武器進捗」に一般化(CustomID・DB キーは pw のまま)
- 募集は **種目ごと** に開ける(`UNIQUE(date, activity_id, detail)`)。同日に
  レベルレとエキルレのカードが並立でき、募集中の種目はセレクトに 📣 印が付く。
  旧 UNIQUE のテーブルは store.Open 時に自動で再構築される
- カード投稿時、やりたい登録者(今日「無理」と開いた本人を除く)へ本文で
  メンション通知する。embed 内のメンションは通知が飛ばないため Content に載せること
- **フリー募集**(`activity.Free`、All には載せない擬似カテゴリ): 入口メッセージと
  サマリーの「✍️ フリー募集」ボタン → モーダルで自由テキスト(60 字)→ 募集カード。
  detail にテキストをそのまま入れ、参加・スレッド作成は通常募集と共通
- **2 階層化・フリー募集の実機検証は未実施**(ユニットテストは通過)。エントリ
  メッセージのボタン(武器進捗・フリー募集)を反映するには `!eab setup` の再実行が必要
- 未実施: 本番ホスティングへのデプロイ(Dockerfile は用意済み)

## 設計上の約束ごと

- CustomID はステートレス(`prop:join:<proposalID>` / `summary:open:<日付>` 形式)。
  再起動後も過去メッセージのボタンが機能する状態を維持すること
- 日付はすべて JST の `YYYY-MM-DD`。`bot.today()` を使う
- アクティビティ・種目・時間帯・スタンスの追加変更は `internal/activity/activity.go` のみ
  (セレクトメニューの仕様上、カテゴリは最大 25 件、種目は「相談して決める」+1 で 25 件。
  上限・ID 重複は activity パッケージのテストが検査する)
- アクティビティ ID の変更・統廃合は DB を書き換えず、`activity.legacyIDs` に
  旧 ID → 新 ID のエイリアスを足して読み出し時に吸収する
- ユーザー向け文言はヒルディブランド調(「ごきげんよう、諸君!」「〜であります」)。
  ボタン・セレクトのラベルは操作性優先で簡潔に、口調は説明文・応答に乗せる
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
- サマリーの同日重複防止(settings の `summary_date`)は **20 時の自動投稿のみ**。
  `!eab post` は管理者の明示操作として常に投稿する(force)
