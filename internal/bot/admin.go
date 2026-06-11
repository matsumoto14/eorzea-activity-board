package bot

import (
	"log/slog"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// onMessageCreate は管理者向け !eab コマンドだけを処理する。
// 一般メンバーはコマンドを一切使わない(すべてボタン操作)。
func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}
	// 語境界を要求する("!eabpost" のようなタイポや "!eabb..." への誤反応を防ぐ)
	if m.Content != "!eab" && !strings.HasPrefix(m.Content, "!eab ") {
		return
	}
	if !b.isAdmin(s, m) {
		_, _ = s.ChannelMessageSend(m.ChannelID, "⚠️ このコマンドはサーバー管理権限(サーバー管理)が必要です。")
		return
	}

	cmd := strings.TrimSpace(strings.TrimPrefix(m.Content, "!eab"))
	var err error
	switch cmd {
	case "setup":
		err = b.cmdSetup(s, m)
	case "post":
		var posted bool
		if posted, err = b.postDailyProposals(); err == nil {
			msg := "📤 今日の候補を投稿しました。"
			if !posted {
				msg = "ℹ️ 今日のサマリーは投稿済みのためスキップしました(チャンネル未設定の場合は `!eab setup`)。"
			}
			_, err = s.ChannelMessageSend(m.ChannelID, msg)
		}
	case "help", "":
		_, err = s.ChannelMessageSend(m.ChannelID, strings.Join([]string{
			"**eorzea-activity-board 管理コマンド**",
			"`!eab setup` … このチャンネルを掲示板に設定し、入口メッセージを設置",
			"`!eab post` … 今日の候補をいますぐ投稿(テスト・臨時用)",
			"`!eab help` … この一覧",
		}, "\n"))
	default:
		_, err = s.ChannelMessageSend(m.ChannelID,
			"⚠️ 不明なコマンドです: `!eab "+cmd+"`。`!eab help` で一覧を確認してください。")
	}
	if err != nil {
		slog.Error("管理コマンド失敗", "cmd", cmd, "err", err)
		_, _ = s.ChannelMessageSend(m.ChannelID, "⚠️ コマンドの実行に失敗しました: "+err.Error())
	}
}

func (b *Bot) isAdmin(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	perms, err := s.UserChannelPermissions(m.Author.ID, m.ChannelID)
	if err != nil {
		slog.Error("権限取得失敗", "err", err)
		return false
	}
	return perms&discordgo.PermissionManageServer != 0 ||
		perms&discordgo.PermissionAdministrator != 0
}

// cmdSetup は実行チャンネルを掲示板として登録し、入口メッセージを設置する。
func (b *Bot) cmdSetup(s *discordgo.Session, m *discordgo.MessageCreate) error {
	if err := b.store.SetSetting(settingBoardChannel, m.ChannelID); err != nil {
		return err
	}
	msgID, err := b.PostEntryMessage(m.ChannelID)
	if err != nil {
		return err
	}
	if err := b.store.SetSetting(settingEntryMessage, msgID); err != nil {
		return err
	}
	_, err = s.ChannelMessageSend(m.ChannelID,
		"✅ このチャンネルを掲示板に設定しました。毎日 20 時ごろに候補を投稿します。")
	return err
}
