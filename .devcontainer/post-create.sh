#!/usr/bin/env bash
set -euo pipefail

# Claude Code 本体は devcontainer feature
# (ghcr.io/anthropics/devcontainer-features/claude-code) でインストール済み。

# ~/.claude と /commandhistory と /go/pkg は名前付きボリューム。初回マウント時に
# root 所有になることがあり、/login の認証情報書き込みや gopls のモジュール
# キャッシュ作成(mkdir /go/pkg/mod)に失敗するため所有者を直す。
sudo chown -R vscode:vscode /home/vscode/.claude /commandhistory
sudo chown -R vscode:golang /go/pkg

# bash 履歴をボリュームに永続化(公式リファレンス実装と同等)
if ! grep -q '/commandhistory/.bash_history' ~/.bashrc; then
  echo 'export PROMPT_COMMAND="history -a" HISTFILE=/commandhistory/.bash_history' >> ~/.bashrc
fi

# 開発補助ツール。
# node feature が追加する yarn リポジトリは GPG 鍵切れで apt-get update を
# 失敗させる(set -e でスクリプトごと死ぬ)ため、先に除去する(yarn は未使用)
sudo rm -f /etc/apt/sources.list.d/yarn.list
sudo apt-get update
sudo apt-get install -y --no-install-recommends sqlite3
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# 依存取得(go.mod があれば)
if [ -f go.mod ]; then
  go mod download
fi

echo "post-create done. 'claude' でClaude Codeを起動できます。"
