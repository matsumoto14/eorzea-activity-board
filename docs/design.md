# eorzea-activity-board 設計

## 全体構成

```
┌─────────────────────────────────────────────┐
│  1プロセスの Go バイナリ(常駐 Gateway Bot)   │
│                                             │
│  discordgo ──── Discord Gateway (WebSocket) │
│  内蔵スケジューラ ── 毎日 20:00 JST に投稿     │
│  SQLite (modernc.org/sqlite, CGO 不要)       │
└─────────────────────────────────────────────┘
```

- 外部サービス依存なし(cron も DB サーバーも不要)。1 コンテナ / 1 バイナリで完結
- HTTP サーバーを持たない(Interaction は Gateway 経由で受信)ため、公開エンドポイント不要
- タイムゾーンは `time/tzdata` を埋め込み、コンテナに tzdata 不要

## パッケージ構成

```
cmd/bot/main.go           エントリポイント(起動・シグナル処理)
internal/config/          環境変数の読み込み
internal/activity/        アクティビティ・時間帯・スタンスの定義(コード内マスタ)
internal/store/           SQLite アクセス(スキーマ・CRUD)
internal/bot/             Discord ハンドラ
  bot.go                  セッション・ルーティング
  entry.go                入口固定メッセージ/設定・今日の状態・PW進捗
  proposal.go             20時投稿・参加ボタン・スレッド作成
  admin.go                !eab 管理コマンド
internal/scheduler/       毎日定時実行
```

## データモデル(SQLite)

```sql
settings      (key TEXT PK, value TEXT)            -- board_channel, entry_message など
user_prefs    (user_id PK, activities, timeslots,  -- カンマ区切りID列
               stance, updated_at)
daily_status  (user_id, date, status)              -- ok / maybe / no, PK(user_id, date)
pw_progress   (user_id PK, progress, updated_at)   -- 自由記述メモ
proposals     (id PK AUTOINCREMENT, date, activity_id, channel_id,
               message_id, thread_id, UNIQUE(date, activity_id))
responses     (proposal_id, user_id, response,     -- join / standby / no
               PK(proposal_id, user_id))
```

- 日付はすべて JST の `YYYY-MM-DD`
- `UNIQUE(date, activity_id)` により同日同活動の二重投稿を防止(手動 post と 20 時の重複対策)

## インタラクション設計(CustomID 体系)

| CustomID | 種別 | 動作 |
|----------|------|------|
| `entry:prefs` | ボタン | エフェメラルで設定用セレクトメニュー 3 つを表示 |
| `entry:today` | ボタン | エフェメラルで今日の状態ボタン 3 つを表示 |
| `entry:pw` | ボタン | PW 進捗入力モーダルを表示 |
| `entry:pwlist` | ボタン | エフェメラルで全員の PW 進捗一覧を表示 |
| `prefs:activities` | 複数選択 | やりたいこと保存(選択即保存) |
| `prefs:timeslots` | 複数選択 | 時間帯保存 |
| `prefs:stance` | 単一選択 | スタンス保存 |
| `today:ok` `today:maybe` `today:no` | ボタン | 今日の状態を保存 |
| `pw:modal` / `pw:progress` | モーダル | PW 進捗保存 |
| `summary:open:<date>` | 単一選択 | サマリーで選ばれた活動の募集カードを立てる(募集中なら既存カードへ案内)。日付が当日でない(過去のサマリー)場合は受け付けない |
| `prop:join:<id>` | ボタン | 参加する |
| `prop:standby:<id>` | ボタン | 人数足りたら呼んで |
| `prop:no:<id>` | ボタン | 今日は無理 |

CustomID はすべてステートレス(必要な情報を ID に埋め込む)ため、
Bot を再起動しても過去メッセージのボタンがそのまま機能する。

## 20 時投稿のロジック(サマリー型)

ノイズを抑えるため、20 時に流すのは **サマリー 1 通だけ**。募集カードは
誰かがサマリーで活動を選んだときに初めて立てる(遅延生成)。

1. 全メンバーの `user_prefs` を取得
2. アクティビティごとに「やりたい登録があり、今日の状態が `no` でない」メンバーを候補化
3. 候補 2 人以上の活動を一覧にした **サマリーを 1 通** 投稿(セレクトメニュー付き)
4. 候補が 1 件もなければ「今日は候補なし」を 1 通だけ投稿
5. サマリー(候補なし含む)は settings の `summary_date` で同日 1 回に制限
   (スケジューラと `!eab post` の重複対策。2 回目の `!eab post` はスキップを通知)
6. メンバーがセレクトで活動を選ぶと、その活動の **募集カード** を投稿
   (`UNIQUE(date, activity_id)` により同日同活動の二重カードは立たない。
   既に募集中の活動を選んだ人には既存カードへのリンクをエフェメラルで案内。
   カード投稿に失敗した場合は提案行を巻き戻し、次の選択で再挑戦できる)

募集カードは進捗フォーカス型: 参加人数の進捗バー(▰▰▱▱)と「あと N 人で自動スレッド」を
主役にし、参加中・呼んで・パス・ふだんやりたいメンバーを下に並べる。
PW のカードには進捗メモも添える。

高度なスコアリングは行わない。「興味のある人が 2 人以上いる」だけで候補に出す。

## 成立(スレッド作成)のロジック

1. 「参加する」押下のたびに join 数を集計
2. join 数 >= アクティビティの成立人数 かつ スレッド未作成なら、その投稿メッセージから
   スレッドを作成(自動アーカイブ 24h)
3. スレッド内で参加者(join)+「呼んで」(standby)メンバーをメンションして相談開始
4. 成立後に追加で「参加する」を押した人はスレッド内に通知
5. ボタンは成立後も有効(出入り自由)。embed は押下のたびに最新状態へ更新

### 堅牢性(ハンドラの約束ごと)

- ボタン/サマリー選択のハンドラは **先に deferred ack** してから処理する
  (Discord の 3 秒応答期限をスレッド作成等の重い処理で破らない)
- 提案の作成・成立判定は Bot 内のミューテックスで直列化
  (しきい値到達時の同時押しによるスレッド二重作成を防ぐ)
- DB とDiscord の不整合は押下時に自己修復する: `message_id` 欠落はボタンの
  付いたメッセージ自身から復元、スレッド作成済みなのに `thread_id` 欠落は
  既存スレッド(メッセージ起点スレッドの ID = 元メッセージ ID)を拾い直す
- embed の field(1024 字)/description(4096 字)は上限でキャップする

## 必要な Discord 設定

- Privileged Gateway Intents: **MESSAGE CONTENT**(`!eab` 管理コマンド用)
- Bot 権限: View Channels / Send Messages / Embed Links / Read Message History /
  Create Public Threads / Send Messages in Threads / Manage Messages(ピン留め用)

## 運用・デプロイ

- 環境変数: `DISCORD_TOKEN`(必須), `EAB_DB_PATH`(既定 `data/eab.db`),
  `EAB_POST_HOUR`/`EAB_POST_MINUTE`(既定 20:00)
- Dockerfile はマルチステージで distroless/static へ。イメージ ~15MB、常駐メモリ ~20MB
- 候補ホスティング: 手元の常時起動 PC / Oracle Cloud Always Free / fly.io 最小構成 /
  さくら VPS 最安プランなど。WebSocket 常駐が許される環境ならどこでも可
- バックアップ: `eab.db` をコピーするだけ
