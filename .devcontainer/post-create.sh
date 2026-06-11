#!/usr/bin/env bash
set -euo pipefail

# Claude Code 本体は devcontainer feature
# (ghcr.io/anthropics/devcontainer-features/claude-code) でインストール済み。

# ~/.claude と /commandhistory は名前付きボリューム。初回マウント時に root 所有に
# なることがあり、/login の認証情報書き込み等に失敗するため所有者を直す。
sudo chown -R vscode:vscode /home/vscode/.claude /commandhistory

# bash 履歴をボリュームに永続化(公式リファレンス実装と同等)
if ! grep -q '/commandhistory/.bash_history' ~/.bashrc; then
  echo 'export PROMPT_COMMAND="history -a" HISTFILE=/commandhistory/.bash_history' >> ~/.bashrc
fi

# 開発補助ツール
sudo apt-get update
sudo apt-get install -y --no-install-recommends sqlite3
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# 依存取得(go.mod があれば)
if [ -f go.mod ]; then
  go mod download
fi

echo "post-create done. 'claude' でClaude Codeを起動できます。"
