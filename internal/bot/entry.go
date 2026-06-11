package bot

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"eorzea-activity-board/internal/activity"
)

// PostEntryMessage は入口の固定メッセージを channelID に投稿してピン留めし、
// メッセージ ID を返す。
func (b *Bot) PostEntryMessage(channelID string) (string, error) {
	embed := &discordgo.MessageEmbed{
		Title: "🎩 エオルゼア活動ボード",
		Description: strings.Join([]string{
			"ごきげんよう、諸君! 私、ヒルディブランド……",
			"人呼んで、**美麗なる紳士** であります!",
			"",
			"このボードは、日々の活動をゆるくマッチングする社交場。",
			"諸君はボタンを押すだけでよいのです。",
			"",
			"───────────────",
			"",
			"**📝 やりたいこと設定**",
			"普段やりたい活動・時間帯・スタンスを登録",
			"",
			"**📅 今日の状態**",
			"「今日いける/呼ばれたら/無理」を切り替え",
			"",
			"**⚔️ 武器進捗を更新**",
			"PW・RW など武器作成の進捗メモを更新",
			"",
			"**📋 武器進捗一覧**",
			"諸君の進捗を一望",
			"",
			"**✍️ フリー募集**",
			"好きな内容を入力して、その場で今日の募集を立てる",
			"",
			"───────────────",
			"",
			"毎晩 20 時ごろ、私が本日の活動候補をお届けします。",
			"人数が揃えば、スレッドも自動で立つ……",
			"さすれば事件(マッチング)は、自ずと解決するのであります!",
		}, "\n"),
		Color: 0xD4AF37,
	}
	msg, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "📝 やりたいこと設定", Style: discordgo.PrimaryButton, CustomID: "entry:prefs"},
				discordgo.Button{Label: "📅 今日の状態", Style: discordgo.SecondaryButton, CustomID: "entry:today"},
				discordgo.Button{Label: "⚔️ 武器進捗を更新", Style: discordgo.SecondaryButton, CustomID: "entry:pw"},
				discordgo.Button{Label: "📋 武器進捗一覧", Style: discordgo.SecondaryButton, CustomID: "entry:pwlist"},
				discordgo.Button{Label: "✍️ フリー募集", Style: discordgo.SecondaryButton, CustomID: "free:open"},
			}},
		},
	})
	if err != nil {
		return "", err
	}
	// ピン留め失敗(権限不足)は致命的ではないので無視する
	_ = b.session.ChannelMessagePin(channelID, msg.ID)
	return msg.ID, nil
}

// handleEntryPrefs はやりたいこと設定のセレクトメニュー 3 つをエフェメラルで返す。
// 保存済みの値を Default として反映する。
func (b *Bot) handleEntryPrefs(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	uid := interactionUser(i).ID
	prefs, err := b.store.GetPrefs(uid)
	if err != nil {
		return err
	}

	// 保存済みの旧アクティビティ ID は現行 ID に解決してから Default に反映する
	savedActs := activity.NormalizeIDs(prefs.Activities)
	actOpts := make([]discordgo.SelectMenuOption, 0, len(activity.All))
	for _, a := range activity.All {
		actOpts = append(actOpts, discordgo.SelectMenuOption{
			Label:   a.Emoji + " " + a.Name,
			Value:   a.ID,
			Default: slices.Contains(savedActs, a.ID),
		})
	}
	slotOpts := make([]discordgo.SelectMenuOption, 0, len(activity.TimeSlots))
	for _, t := range activity.TimeSlots {
		slotOpts = append(slotOpts, discordgo.SelectMenuOption{
			Label:   t.Label,
			Value:   t.ID,
			Default: slices.Contains(prefs.TimeSlots, t.ID),
		})
	}
	stanceOpts := make([]discordgo.SelectMenuOption, 0, len(activity.Stances))
	for _, st := range activity.Stances {
		stanceOpts = append(stanceOpts, discordgo.SelectMenuOption{
			Label:   st.Label,
			Value:   st.ID,
			Default: prefs.Stance == st.ID,
		})
	}

	zero := 0
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				MenuType:    discordgo.StringSelectMenu,
				CustomID:    "prefs:activities",
				Placeholder: "普段やりたいこと(複数選択可)",
				MinValues:   &zero,
				MaxValues:   len(actOpts),
				Options:     actOpts,
			},
		}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				MenuType:    discordgo.StringSelectMenu,
				CustomID:    "prefs:timeslots",
				Placeholder: "参加しやすい時間帯(複数選択可)",
				MinValues:   &zero,
				MaxValues:   len(slotOpts),
				Options:     slotOpts,
			},
		}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				MenuType:    discordgo.StringSelectMenu,
				CustomID:    "prefs:stance",
				Placeholder: "参加スタンス",
				Options:     stanceOpts,
			},
		}},
	}
	return respondEphemeral(s, i,
		"📝 **やりたいこと設定** ─ お好みをお聞かせください(選んだ瞬間に保存されます)", components, nil)
}

// handlePrefsSelect はセレクトメニューの選択を即保存する。
func (b *Bot) handlePrefsSelect(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) error {
	uid := interactionUser(i).ID
	values := i.MessageComponentData().Values

	var err error
	switch customID {
	case "prefs:activities":
		err = b.store.SetPrefActivities(uid, values)
	case "prefs:timeslots":
		err = b.store.SetPrefTimeSlots(uid, values)
	case "prefs:stance":
		stance := ""
		if len(values) > 0 {
			stance = values[0]
		}
		err = b.store.SetPrefStance(uid, stance)
	}
	if err != nil {
		return err
	}
	// 表示はそのまま、応答だけ返す
	return ackUpdate(s, i)
}

var todayStatuses = []struct {
	ID    string
	Label string
	Style discordgo.ButtonStyle
}{
	{ID: "ok", Label: "✅ 今日いける", Style: discordgo.SuccessButton},
	{ID: "maybe", Label: "🔔 呼ばれたら", Style: discordgo.SecondaryButton},
	{ID: "no", Label: "😴 今日は無理", Style: discordgo.DangerButton},
}

func todayStatusLabel(id string) string {
	for _, st := range todayStatuses {
		if st.ID == id {
			return st.Label
		}
	}
	return id
}

func todayButtons() []discordgo.MessageComponent {
	buttons := make([]discordgo.MessageComponent, 0, len(todayStatuses))
	for _, st := range todayStatuses {
		buttons = append(buttons, discordgo.Button{
			Label: st.Label, Style: st.Style, CustomID: "today:" + st.ID,
		})
	}
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: buttons}}
}

// handleEntryToday は今日の状態変更ボタンをエフェメラルで返す。
func (b *Bot) handleEntryToday(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return respondEphemeral(s, i,
		fmt.Sprintf("📅 **本日(%s)のご予定** はいかがかな?", b.todayLabel()),
		todayButtons(), nil)
}

// handleTodayButton は今日の状態を保存し、エフェメラルの表示を更新する。
func (b *Bot) handleTodayButton(s *discordgo.Session, i *discordgo.InteractionCreate, status string) error {
	uid := interactionUser(i).ID
	if err := b.store.SetDailyStatus(uid, b.today(), status); err != nil {
		return err
	}
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("📅 本日(%s)の状態、**%s** にて承りました!(いつでも変更できます)",
				b.todayLabel(), todayStatusLabel(status)),
			Components: todayButtons(),
		},
	})
}

// handleEntryPW は PW 進捗入力モーダルを開く。
func (b *Bot) handleEntryPW(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	uid := interactionUser(i).ID
	current, err := b.store.GetPWProgress(uid)
	if err != nil {
		return err
	}
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "pw:modal",
			Title:    "武器作成の進捗",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    "pw:progress",
						Label:       "いまの進捗",
						Style:       discordgo.TextInputShort,
						Placeholder: "例: PW 2本目 / RW 素材集め中 / ZW あと3個",
						Value:       current,
						Required:    true,
						MaxLength:   100,
					},
				}},
			},
		},
	})
}

// handlePWModalSubmit はモーダルの入力を保存する。
func (b *Bot) handlePWModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	uid := interactionUser(i).ID
	progress := modalTextInput(i.ModalSubmitData(), "pw:progress")
	if err := b.store.SetPWProgress(uid, progress); err != nil {
		return err
	}
	return respondEphemeral(s, i,
		fmt.Sprintf("⚔️ 武器進捗、確かに記録いたしました: **%s**", progress), nil, nil)
}

// handleEntryPWList は全員の PW 進捗一覧をエフェメラルで返す。
func (b *Bot) handleEntryPWList(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	list, err := b.store.GetAllPWProgress()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return respondEphemeral(s, i,
			"📋 ふむ、まだ誰も武器進捗を登録していないようであります。「⚔️ 武器進捗を更新」からどうぞ。", nil, nil)
	}
	var sb strings.Builder
	for _, p := range list {
		date := ""
		if t, err := time.Parse(time.RFC3339, p.UpdatedAt); err == nil {
			date = fmt.Sprintf("(%s 更新)", monthDay(t.In(b.loc)))
		}
		// 1 人 2 行 + 空行で、詰まらない一覧にする
		fmt.Fprintf(&sb, "<@%s>\n　└ %s %s\n\n", p.UserID, p.Progress, date)
	}
	embed := &discordgo.MessageEmbed{
		Title:       "📋 武器作成 進捗一覧",
		Description: limitEmbed(sb.String(), 4096), // description の上限は 4096 文字
		Color:       0xE67E22,
		Footer:      &discordgo.MessageEmbedFooter{Text: "諸君の研鑽、見事であります"},
	}
	return respondEphemeral(s, i, "", nil, []*discordgo.MessageEmbed{embed})
}
