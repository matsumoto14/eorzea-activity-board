# eorzea-activity-board

身内の FF14 Discord グループ向けに、日々の活動(レベルレ・モブハン・PW作成・地図)を
ゆるくマッチングする低コスト Bot。

毎日 20 時ごろに「今日これ成立しそう」という候補を投稿し、メンバーはボタンを
押すだけ。人数が集まったら Bot が自動でスレッドを作って PT 相談が始まります。
募集文も Slash Command も不要です。

- 要件: [docs/requirements.md](docs/requirements.md)
- 設計: [docs/design.md](docs/design.md)

## 開発(Devcontainer)

開発は Devcontainer 内の Claude Code で行います。

1. VS Code でこのリポジトリを開く → 「Reopen in Container」
2. 初回は post-create で Go ツール・Claude Code・sqlite3 が自動セットアップされる
3. コンテナ内ターミナルで `claude` を起動して開発を進める

```bash
# コンテナ内での基本コマンド
go mod tidy
go build ./...
DISCORD_TOKEN=... go run ./cmd/bot
```

ホストの `DISCORD_TOKEN` / `ANTHROPIC_API_KEY` 環境変数はコンテナへ引き継がれます。

## Discord 側の準備

1. [Discord Developer Portal](https://discord.com/developers/applications) でアプリ作成 → Bot のトークンを取得
2. **Privileged Gateway Intents で「MESSAGE CONTENT INTENT」を有効化**(管理コマンド用)
3. OAuth2 URL Generator: scope `bot`、権限は
   View Channels / Send Messages / Embed Links / Read Message History /
   Create Public Threads / Send Messages in Threads / Manage Messages
4. 生成された URL でサーバーに招待
5. 掲示板にしたいチャンネルで `!eab setup` を実行(サーバー管理権限が必要)

## 運用

```bash
docker build -t eab .
docker run -d --name eab -e DISCORD_TOKEN=... -v eab-data:/data --restart unless-stopped eab
```

- データは `/data/eab.db`(SQLite)1 ファイルのみ。バックアップはコピーするだけ
- 投稿時刻は `EAB_POST_HOUR` / `EAB_POST_MINUTE` で変更可(既定 20:00 JST)
- 常駐メモリ ~20MB。最小 VPS / 無料枠で動作する想定

## 管理コマンド(サーバー管理権限者のみ)

| コマンド | 動作 |
|---|---|
| `!eab setup` | 実行チャンネルを掲示板に設定し、入口の固定メッセージを設置・ピン留め |
| `!eab post` | 今日の候補をいますぐ投稿(テスト・臨時用) |
| `!eab help` | コマンド一覧 |
