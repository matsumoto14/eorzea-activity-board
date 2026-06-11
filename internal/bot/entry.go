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
		Title: "🏰 エオルゼア活動ボード",
		Description: strings.Join([]string{
			"日々の活動をゆるくマッチングするボードです。",
			"",
			"**📝 やりたいこと設定** … 普段やりたい活動・時間帯・スタンスを登録",
			"**📅 今日の状態** … 「今日いける/呼ばれたら/無理」を切り替え",
			"**⚔️ PW進捗を更新** … 自分の PW 作成の進捗メモを更新",
			"**📋 PW進捗一覧** … みんなの進捗を見る",
			"",
			"毎日 20 時ごろ、成立しそうな活動の候補を投稿します。",
			"候補にはボタンで反応するだけで OK。人数が集まったら自動でスレッドが立ちます。",
		}, "\n"),
		Color: 0x5865F2,
	}
	msg, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "📝 やりたいこと設定", Style: discordgo.PrimaryButton, CustomID: "entry:prefs"},
				discordgo.Button{Label: "📅 今日の状態", Style: discordgo.SecondaryButton, CustomID: "entry:today"},
				discordgo.Button{Label: "⚔️ PW進捗を更新", Style: discordgo.SecondaryButton, CustomID: "entry:pw"},
				discordgo.Button{Label: "📋 PW進捗一覧", Style: discordgo.SecondaryButton, CustomID: "entry:pwlist"},
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

	actOpts := make([]discordgo.SelectMenuOption, 0, len(activity.All))
	for _, a := range activity.All {
		actOpts = append(actOpts, discordgo.SelectMenuOption{
			Label:   a.Emoji + " " + a.Name,
			Value:   a.ID,
			Default: slices.Contains(prefs.Activities, a.ID),
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
		"📝 **やりたいこと設定**(選んだ瞬間に保存されます)", components, nil)
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
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
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
		fmt.Sprintf("📅 **今日(%s)の状態**を選んでください", b.todayLabel()),
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
			Content: fmt.Sprintf("📅 今日(%s)の状態を **%s** にしました(いつでも変更できます)",
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
			Title:    "PW作成の進捗",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    "pw:progress",
						Label:       "いまの進捗",
						Style:       discordgo.TextInputShort,
						Placeholder: "例: 2本目 / 素材集め中 / 強化素材あと3個",
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
	data := i.ModalSubmitData()
	progress := ""
	for _, row := range data.Components {
		ar, ok := row.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, c := range ar.Components {
			if ti, ok := c.(*discordgo.TextInput); ok && ti.CustomID == "pw:progress" {
				progress = ti.Value
			}
		}
	}
	if err := b.store.SetPWProgress(uid, progress); err != nil {
		return err
	}
	return respondEphemeral(s, i,
		fmt.Sprintf("⚔️ PW進捗を更新しました: **%s**", progress), nil, nil)
}

// handleEntryPWList は全員の PW 進捗一覧をエフェメラルで返す。
func (b *Bot) handleEntryPWList(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	list, err := b.store.GetAllPWProgress()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return respondEphemeral(s, i,
			"📋 まだ誰も PW 進捗を登録していません。「⚔️ PW進捗を更新」から登録できます。", nil, nil)
	}
	var sb strings.Builder
	for _, p := range list {
		date := ""
		if t, err := time.Parse(time.RFC3339, p.UpdatedAt); err == nil {
			date = fmt.Sprintf("(%s 更新)", monthDay(t.In(b.loc)))
		}
		fmt.Fprintf(&sb, "<@%s> … %s %s\n", p.UserID, p.Progress, date)
	}
	embed := &discordgo.MessageEmbed{
		Title:       "📋 PW作成 進捗一覧",
		Description: limitEmbed(sb.String(), 4096), // description の上限は 4096 文字
		Color:       0xE67E22,
	}
	return respondEphemeral(s, i, "", nil, []*discordgo.MessageEmbed{embed})
}
