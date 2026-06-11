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
pw_progress   (user_id PK, progress, updated_at)   -- 武器作成の自由記述メモ
proposals     (id PK AUTOINCREMENT, date, activity_id, detail, channel_id,
               message_id, thread_id, UNIQUE(date, activity_id, detail))
                                                   -- detail は種目 ID。フリー募集は
                                                   -- ユーザー入力テキスト(未指定は '')
responses     (proposal_id, user_id, response,     -- join / standby / no
               PK(proposal_id, user_id))
```

- 日付はすべて JST の `YYYY-MM-DD`
- `UNIQUE(date, activity_id, detail)` により**同日同種目**の二重カードだけを防ぐ。
  同日同カテゴリでも種目が違えば別の募集を開ける
  (例: ルーレット(レベルレ)とルーレット(エキルレ)は同日に並立できる)
- 旧アクティビティ ID(levelre / pw など)は `activity.NormalizeIDs` / `ByID` が
  読み出し時に現行 ID へ解決する(DB の書き換えマイグレーションはしない)。
  proposals だけは Open 時に素朴なマイグレーションを行う:
  `detail` 列の追加(`ALTER TABLE`、duplicate column は無視)と、
  旧 `UNIQUE(date, activity_id)` テーブルの再構築(sqlite_master の SQL 文字列で
  新旧を判定し、トランザクション内で RENAME → CREATE → INSERT → DROP)

## インタラクション設計(CustomID 体系)

| CustomID | 種別 | 動作 |
|----------|------|------|
| `entry:prefs` | ボタン | エフェメラルで設定用セレクトメニュー 3 つを表示 |
| `entry:today` | ボタン | エフェメラルで今日の状態ボタン 3 つを表示 |
| `entry:pw` | ボタン | 武器作成の進捗入力モーダルを表示 |
| `entry:pwlist` | ボタン | エフェメラルで全員の武器進捗一覧を表示 |
| `prefs:activities` | 複数選択 | やりたいこと保存(選択即保存) |
| `prefs:timeslots` | 複数選択 | 時間帯保存 |
| `prefs:stance` | 単一選択 | スタンス保存 |
| `today:ok` `today:maybe` `today:no` | ボタン | 今日の状態を保存 |
| `pw:modal` / `pw:progress` | モーダル | 武器進捗保存 |
| `summary:open:<date>` | 単一選択 | サマリーで選ばれた活動の募集を開く。種目があるカテゴリは種目セレクトをエフェメラルで表示(募集中の種目には 📣 印)、無いカテゴリは募集中なら既存カードへ案内・無ければ即カードを立てる。日付が当日でない(過去のサマリー)場合は受け付けない |
| `summary:variant:<date>:<activityID>` | 単一選択 | 種目(「相談して決める」含む)を確定して募集カードを立てる(その種目が募集中なら既存カードへ案内)。日付が変わっていたら受け付けない |
| `free:open` | ボタン | フリー募集の入力モーダルを開く(入口メッセージとサマリーの両方に設置。日付は送信時の「今日」) |
| `free:modal` / `free:text` | モーダル | 入力テキスト(60 字まで)を detail にしてフリー募集カードを立てる |
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
3. **全活動をセレクトメニューに載せたサマリーを 1 通** 投稿。やりたい人が 2 人以上の
   活動は本文で強調表示し、メニューはやりたい人が多い順に並べる
   (候補が少なくても誰でも募集を開ける。「候補なし=何もできない」を作らない)
4. **20 時の自動投稿だけ** settings の `summary_date` で同日 1 回に制限する
   (手動 post 済みの日に自動投稿が被らないようにするため)。
   `!eab post` は管理者の明示操作なので、何度でも投稿できる(強制投稿)
5. メンバーがセレクトで活動を選ぶと募集を開く。種目があるカテゴリは
   **種目セレクト(エフェメラル)を 1 ステップだけ挟む**: 先頭は「🤝 内容は相談して決める」で
   種目の確定を強制しない。募集中の種目には 📣 印を付ける(選ぶと既存カードへ案内)
6. 種目確定(または種目なしカテゴリの選択)で **募集カード** を投稿。
   募集は **種目ごと** に開ける(同日にレベルレとエキルレのカードが並立できる)。
   カード投稿時、そのカテゴリをやりたい登録しているメンバー
   (今日「無理」の人と募集を開いた本人は除く)へ本文でメンション通知する
   (embed 内のメンションは通知が飛ばないため Content に載せる)
   (`UNIQUE(date, activity_id, detail)` により同日同種目の二重カードは立たない。
   既に募集中の種目を選んだ人には既存カードへのリンクをエフェメラルで案内。
   カード投稿に失敗した場合は提案行を巻き戻し、次の選択で再挑戦できる)
7. カテゴリに収まらない内容は **フリー募集**: 入口メッセージ・サマリーの
   「✍️ フリー募集」ボタンからモーダルで自由テキスト(60 字まで)を入力すると、
   そのテキストをタイトルにした募集カードが立つ(参加ボタン・スレッド作成は共通)

募集カードは進捗フォーカス型: 参加人数の進捗バー(▰▰▱▱)と「あと N 人で自動スレッド」を
主役にし、参加中・呼んで・パス・ふだんやりたいメンバーを下に並べる。
カードのタイトルとスレッド名には種目を「地図(G17)」形式で載せる。
武器作成のカードには進捗メモも添える。

高度なスコアリングは行わない。「興味のある人が 2 人以上いる」だけで強調表示する。

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
