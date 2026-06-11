// Package bot は Discord とのやり取り(ハンドラ・UI 構築)をまとめる。
package bot

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"eorzea-activity-board/internal/store"
)

// settings テーブルのキー
const (
	settingBoardChannel = "board_channel"
	settingEntryMessage = "entry_message"
)

type Bot struct {
	session *discordgo.Session
	store   *store.Store
	loc     *time.Location

	// propMu は提案の作成・成立(スレッド作成)を直列化する。
	// discordgo はハンドラを goroutine ごとに走らせるため、しきい値到達時の
	// 同時押しで check-then-act が競合しないようにここで守る。
	propMu sync.Mutex
}

func New(token string, st *store.Store, loc *time.Location) (*Bot, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	// MESSAGE CONTENT は !eab 管理コマンドのために必要
	s.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentMessageContent

	b := &Bot{session: s, store: st, loc: loc}
	s.AddHandler(b.onReady)
	s.AddHandler(b.onGuildCreate)
	s.AddHandler(b.onMessageCreate)
	s.AddHandler(b.onInteraction)
	return b, nil
}

func (b *Bot) Open() error  { return b.session.Open() }
func (b *Bot) Close() error { return b.session.Close() }

func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	slog.Info("Discord 接続完了", "user", r.User.Username, "guilds", len(r.Guilds))
	if len(r.Guilds) == 0 {
		slog.Warn("どのサーバーにも参加していません。招待 URL でテストサーバーへ追加してください")
	}
}

// onGuildCreate は参加中ギルドの確認用。起動直後に在籍ギルド分発火するため、
// 「ちゃんと招待できているか」を起動ログで即確認できる。
func (b *Bot) onGuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	slog.Info("ギルド在籍を確認", "name", g.Name, "id", g.ID)
}

// today は JST の今日 (YYYY-MM-DD)。
func (b *Bot) today() string {
	return time.Now().In(b.loc).Format("2006-01-02")
}

// monthDay は表示用の M/D。
func monthDay(t time.Time) string {
	return fmt.Sprintf("%d/%d", int(t.Month()), t.Day())
}

// todayLabel は今日の表示用 (M/D)。
func (b *Bot) todayLabel() string {
	return monthDay(time.Now().In(b.loc))
}

// interactionUser はギルド内・DM どちらの Interaction からもユーザーを取り出す。
func interactionUser(i *discordgo.InteractionCreate) *discordgo.User {
	if i.Member != nil {
		return i.Member.User
	}
	return i.User
}

// onInteraction は CustomID で各ハンドラへルーティングする。
func (b *Bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var err error
	switch i.Type {
	case discordgo.InteractionMessageComponent:
		id := i.MessageComponentData().CustomID
		switch {
		case id == "entry:prefs":
			err = b.handleEntryPrefs(s, i)
		case id == "entry:today":
			err = b.handleEntryToday(s, i)
		case id == "entry:pw":
			err = b.handleEntryPW(s, i)
		case id == "entry:pwlist":
			err = b.handleEntryPWList(s, i)
		case id == "prefs:activities", id == "prefs:timeslots", id == "prefs:stance":
			err = b.handlePrefsSelect(s, i, id)
		case strings.HasPrefix(id, "summary:open"):
			err = b.handleSummaryOpen(s, i, id)
		case strings.HasPrefix(id, "today:"):
			err = b.handleTodayButton(s, i, strings.TrimPrefix(id, "today:"))
		case strings.HasPrefix(id, "prop:"):
			err = b.handleProposalButton(s, i, id)
		}
	case discordgo.InteractionModalSubmit:
		if i.ModalSubmitData().CustomID == "pw:modal" {
			err = b.handlePWModalSubmit(s, i)
		}
	}
	if err != nil {
		slog.Error("interaction 処理失敗", "err", err)
		// 失敗をユーザーにも伝える。deferred ack 済みのハンドラでは
		// InteractionRespond が通らないため、フォローアップで送る。
		const failMsg = "⚠️ 処理に失敗しました。もう一度試してみてください。"
		respondErr := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: failMsg,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		if respondErr != nil {
			_, _ = s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
				Content: failMsg,
				Flags:   discordgo.MessageFlagsEphemeral,
			})
		}
	}
}

// limitEmbed は Discord の embed 文字数上限に収める(超過分は省略表記)。
// field value は 1024、description は 4096 が上限。
func limitEmbed(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit-1]) + "…"
}

// respondEphemeral はエフェメラル(本人にだけ見える)メッセージで応答する。
func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent, embeds []*discordgo.MessageEmbed) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    content,
			Components: components,
			Embeds:     embeds,
			Flags:      discordgo.MessageFlagsEphemeral,
		},
	})
}

func mentionList(ids []string) string {
	if len(ids) == 0 {
		return "—"
	}
	parts := make([]string, len(ids))
	for n, id := range ids {
		parts[n] = "<@" + id + ">"
	}
	return strings.Join(parts, " ")
}
