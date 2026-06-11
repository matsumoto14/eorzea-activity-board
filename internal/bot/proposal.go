package bot

import (
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"eorzea-activity-board/internal/activity"
	"eorzea-activity-board/internal/store"
)

// 候補として投稿する最低人数(やりたい登録があるメンバー数)
const minCandidates = 2

// settings テーブルのキー: サマリーを投稿済みの日付(同日重複投稿の防止)
const settingSummaryDate = "summary_date"

// PostDailyProposals は今日の候補を掲示板チャンネルへ投稿する(スケジューラ用)。
func (b *Bot) PostDailyProposals() {
	if _, err := b.postDailyProposals(); err != nil {
		slog.Error("20時投稿に失敗", "err", err)
	}
}

// postDailyProposals はサマリーを投稿する。スケジューラと !eab post の両方から
// 呼ばれるため、同日 2 回目以降はスキップして posted=false を返す。
func (b *Bot) postDailyProposals() (bool, error) {
	channelID, err := b.store.GetSetting(settingBoardChannel)
	if err != nil {
		return false, err
	}
	if channelID == "" {
		slog.Warn("掲示板チャンネル未設定のため投稿をスキップ(!eab setup を実行してください)")
		return false, nil
	}

	date := b.today()
	if last, err := b.store.GetSetting(settingSummaryDate); err != nil {
		return false, err
	} else if last == date {
		slog.Info("今日のサマリーは投稿済みのためスキップ", "date", date)
		return false, nil
	}

	prefsAll, err := b.store.GetAllPrefs()
	if err != nil {
		return false, err
	}
	statuses, err := b.store.GetDailyStatuses(date)
	if err != nil {
		return false, err
	}

	// 活動ごとに候補数を数え、最低人数を満たすものだけサマリーに載せる。
	// 募集カードはここでは作らず、誰かがサマリーで選んだ時に遅延生成する。
	var summary []summaryItem
	for _, act := range activity.All {
		candidates := candidatesFor(act.ID, prefsAll, statuses)
		if len(candidates) < minCandidates {
			continue
		}
		summary = append(summary, summaryItem{Activity: act, Count: len(candidates)})
	}

	if len(summary) == 0 {
		if _, err := b.session.ChannelMessageSend(channelID,
			"🌙 今日は成立しそうな候補がありませんでした。「📝 やりたいこと設定」が増えると候補が出やすくなります。"); err != nil {
			return false, err
		}
		slog.Info("候補なしを投稿", "date", date)
	} else {
		if err := b.postSummary(channelID, date, summary); err != nil {
			return false, err
		}
		slog.Info("活動サマリーを投稿", "date", date, "activities", len(summary))
	}
	return true, b.store.SetSetting(settingSummaryDate, date)
}

// summaryItem はサマリー1行ぶん(活動と候補人数)。
type summaryItem struct {
	Activity activity.Activity
	Count    int
}

// postSummary は「今日の活動サマリー」を 1 通だけ投稿する。
// セレクトメニューで活動を選ぶと、その活動の募集が始まる(遅延生成)。
// CustomID に日付を埋め込み、過去のサマリーからの操作を無効化できるようにする。
func (b *Bot) postSummary(channelID, date string, items []summaryItem) error {
	var desc strings.Builder
	desc.WriteString("気になる活動を選ぶと募集が始まります。押すだけ・書き込み不要。\n\n")
	opts := make([]discordgo.SelectMenuOption, 0, len(items))
	for _, it := range items {
		fmt.Fprintf(&desc, "%s **%s** … ふだんやりたい %d人\n", it.Activity.Emoji, it.Activity.Name, it.Count)
		opts = append(opts, discordgo.SelectMenuOption{
			Label:       fmt.Sprintf("%s %s(%d人)", it.Activity.Emoji, it.Activity.Name, it.Count),
			Value:       it.Activity.ID,
			Description: fmt.Sprintf("成立%d人でスレッド自動作成", it.Activity.Threshold),
		})
	}
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🌙 %s の活動、どれやる?", b.todayLabel()),
		Description: desc.String(),
		Color:       0x5865F2,
		Footer:      &discordgo.MessageEmbedFooter{Text: "選ぶと募集カードが立ちます / 何度でも選び直せます"},
	}
	_, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:    discordgo.StringSelectMenu,
					CustomID:    "summary:open:" + date,
					Placeholder: "▼ 今日やりたい活動を選ぶ",
					Options:     opts,
				},
			}},
		},
	})
	return err
}

// handleSummaryOpen はサマリーで選ばれた活動の募集カードを立てる。
// customID は "summary:open:<日付>"。初めて選ばれた活動なら新規にカードを投稿し、
// 既に募集中なら既存カードへ案内する。
func (b *Bot) handleSummaryOpen(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) error {
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		})
	}
	activityID := values[0]
	act, ok := activity.ByID(activityID)
	if !ok {
		return fmt.Errorf("不明なアクティビティ: %s", activityID)
	}

	// 過去のサマリー(別の日付)からの操作は、当日の候補状況と無関係な
	// カードが立ってしまうため受け付けない
	parts := strings.Split(customID, ":")
	if len(parts) != 3 || parts[2] != b.today() {
		return respondEphemeral(s, i,
			"🌙 これは過去のサマリーです。今日のサマリー(毎日20時ごろ投稿)から選んでね。", nil, nil)
	}
	date := parts[2]

	// カード投稿(Discord API)が遅れても interaction を失敗させないよう先に ack する
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	}); err != nil {
		return err
	}
	editReply := func(content string) error {
		_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return err
	}

	channelID, err := b.store.GetSetting(settingBoardChannel)
	if err != nil {
		return err
	}

	// 同時選択でカードが二重投稿されないよう直列化する
	b.propMu.Lock()
	defer b.propMu.Unlock()

	prop, created, err := b.store.CreateProposal(date, activityID)
	if err != nil {
		return err
	}

	// 新規、または以前のカード投稿が失敗して行だけ残った場合 → カードを(再)投稿
	if created || prop.MessageID == "" {
		candidates, err := b.currentCandidates(prop)
		if err != nil {
			if created {
				_ = b.store.DeleteProposal(prop.ID)
			}
			return err
		}
		msgID, err := b.postProposal(channelID, prop.ID, act, candidates)
		if err != nil {
			// カード未送信のまま行だけ残すと、その活動が一日中
			// 「募集中なのにカードが無い」状態で詰むため巻き戻す。
			// 送信済みで DB 更新だけ失敗した場合はカードのボタン押下時に自己修復される。
			if created && msgID == "" {
				if derr := b.store.DeleteProposal(prop.ID); derr != nil {
					slog.Error("提案の巻き戻しに失敗", "id", prop.ID, "err", derr)
				}
			}
			return err
		}
		slog.Info("募集カードを作成", "activity", activityID, "by", interactionUser(i).ID)
		return editReply(fmt.Sprintf("%s **%s** の募集を開きました! カードの「✋ 参加する」を押してね。", act.Emoji, act.Name))
	}

	// すでに募集中 → 既存カードへのリンクを案内する
	link := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", i.GuildID, prop.ChannelID, prop.MessageID)
	return editReply(fmt.Sprintf("%s **%s** はもう募集中です → %s", act.Emoji, act.Name, link))
}

// candidatesFor は act をやりたい登録していて、今日「無理」でないメンバーを返す。
func candidatesFor(actID string, prefsAll []store.Prefs, statuses map[string]string) []store.Prefs {
	var out []store.Prefs
	for _, p := range prefsAll {
		if !slices.Contains(p.Activities, actID) {
			continue
		}
		if statuses[p.UserID] == "no" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// postProposal は募集カードを投稿する。送信できた場合はメッセージ ID を返す
// (送信成功後に DB 更新で失敗した場合、msgID != "" かつ err != nil になる)。
func (b *Bot) postProposal(channelID string, proposalID int64, act activity.Activity, candidates []store.Prefs) (string, error) {
	embed, err := b.buildProposalEmbed(act, candidates, map[string][]string{}, "")
	if err != nil {
		return "", err
	}
	msg, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: proposalButtons(proposalID),
	})
	if err != nil {
		return "", err
	}
	return msg.ID, b.store.SetProposalMessage(proposalID, channelID, msg.ID)
}

func proposalButtons(proposalID int64) []discordgo.MessageComponent {
	id := strconv.FormatInt(proposalID, 10)
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "✋ 参加する", Style: discordgo.SuccessButton, CustomID: "prop:join:" + id},
			discordgo.Button{Label: "🔔 人数足りたら呼んで", Style: discordgo.SecondaryButton, CustomID: "prop:standby:" + id},
			discordgo.Button{Label: "😴 今日は無理", Style: discordgo.SecondaryButton, CustomID: "prop:no:" + id},
		}},
	}
}

// progressBar は参加人数の進捗を ▰▱ のバーで表す。
func progressBar(joins, threshold int) string {
	threshold = max(threshold, 1)
	filled := min(joins, threshold)
	return strings.Repeat("▰", filled) + strings.Repeat("▱", threshold-filled)
}

// buildProposalEmbed は募集カードの embed を組み立てる(進捗フォーカス型)。
// 参加状況が変わるたびに呼び直して最新化する。
func (b *Bot) buildProposalEmbed(act activity.Activity, candidates []store.Prefs, responses map[string][]string, threadID string) (*discordgo.MessageEmbed, error) {
	joins := responses["join"]
	standbys := responses["standby"]
	nos := responses["no"]

	var title, desc string
	color := 0x3498DB
	if threadID != "" {
		title = fmt.Sprintf("%s %s  🎉 成立!", act.Emoji, act.Name)
		desc = fmt.Sprintf("相談はこちら → <#%s>", threadID)
		color = 0x2ECC71
	} else {
		title = fmt.Sprintf("%s %s ─ 今日やる人募集!", act.Emoji, act.Name)
		remaining := max(act.Threshold-len(joins), 0)
		desc = fmt.Sprintf("%s  ✋ 参加 %d / %d人\nあと%d人で自動スレッド 🚀",
			progressBar(len(joins), act.Threshold), len(joins), act.Threshold, remaining)
	}

	// 候補メンバーは時間帯・スタンス付きで表示し、相談の材料にする
	var interested strings.Builder
	for _, p := range candidates {
		var info []string
		if slots := activity.TimeSlotLabels(p.TimeSlots); len(slots) > 0 {
			info = append(info, strings.Join(slots, "・"))
		}
		if l := activity.StanceLabel(p.Stance); l != "" {
			info = append(info, l)
		}
		if len(info) > 0 {
			fmt.Fprintf(&interested, "<@%s>(%s)\n", p.UserID, strings.Join(info, " / "))
		} else {
			fmt.Fprintf(&interested, "<@%s>\n", p.UserID)
		}
	}
	interestedVal := "—"
	if interested.Len() > 0 {
		interestedVal = interested.String()
	}

	// embed field value は 1024 文字が上限。超えると押下のたびに更新が失敗するためキャップする
	const fieldLimit = 1024
	fields := []*discordgo.MessageEmbedField{
		{Name: "✋ 参加中", Value: limitEmbed(mentionList(joins), fieldLimit), Inline: true},
		{Name: "🔔 呼んで", Value: limitEmbed(mentionList(standbys), fieldLimit), Inline: true},
		{Name: "😴 パス", Value: limitEmbed(mentionList(nos), fieldLimit), Inline: true},
		{Name: "💭 ふだんやりたい", Value: limitEmbed(interestedVal, fieldLimit)},
	}

	// PW は進捗メモも相談材料として添える
	if act.ID == "pw" {
		progressList, err := b.store.GetAllPWProgress()
		if err != nil {
			return nil, err
		}
		byUser := map[string]string{}
		for _, p := range progressList {
			byUser[p.UserID] = p.Progress
		}
		var sb strings.Builder
		for _, p := range candidates {
			if prog, ok := byUser[p.UserID]; ok {
				fmt.Fprintf(&sb, "<@%s> … %s\n", p.UserID, prog)
			}
		}
		if sb.Len() > 0 {
			fields = append(fields, &discordgo.MessageEmbedField{
				Name: "⚔️ PW進捗", Value: limitEmbed(sb.String(), fieldLimit),
			})
		}
	}

	return &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Color:       color,
		Fields:      fields,
		Footer:      &discordgo.MessageEmbedFooter{Text: b.todayLabel() + " ・ いつでも押し直しOK"},
	}, nil
}

// handleProposalButton は「参加する/呼んで/無理」の押下を処理する。
// customID は "prop:<resp>:<proposalID>"。
func (b *Bot) handleProposalButton(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) error {
	parts := strings.Split(customID, ":")
	if len(parts) != 3 {
		return fmt.Errorf("不正な CustomID: %s", customID)
	}
	resp := parts[1]
	proposalID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return err
	}
	uid := interactionUser(i).ID

	// スレッド作成など重い処理が挟まっても 3 秒の応答期限を破らないよう先に ack する
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); err != nil {
		return err
	}

	// しきい値到達時の同時押しでスレッドが二重作成されないよう直列化する
	b.propMu.Lock()
	defer b.propMu.Unlock()

	// 先に提案の存在を確認してから回答を記録する(存在しない提案への孤児行を作らない)
	prop, err := b.store.GetProposal(proposalID)
	if err != nil {
		return err
	}
	act, ok := activity.ByID(prop.ActivityID)
	if !ok {
		return fmt.Errorf("不明なアクティビティ: %s", prop.ActivityID)
	}
	prev, err := b.store.SetResponse(proposalID, uid, resp)
	if err != nil {
		return err
	}
	responses, err := b.store.GetResponses(proposalID)
	if err != nil {
		return err
	}

	// ボタンが付いているこのメッセージ自体が募集カード。過去の障害などで
	// DB に channel/message_id が残っていなければ、ここで自己修復する
	if prop.MessageID == "" && i.Message != nil {
		prop.ChannelID, prop.MessageID = i.ChannelID, i.Message.ID
		if err := b.store.SetProposalMessage(prop.ID, prop.ChannelID, prop.MessageID); err != nil {
			return err
		}
	}

	// 成立判定: 参加する が しきい値以上 かつ スレッド未作成
	joins := responses["join"]
	if prop.ThreadID == "" && len(joins) >= act.Threshold {
		threadID, err := b.establishProposal(s, prop, act, responses)
		if err != nil {
			return err
		}
		prop.ThreadID = threadID
	} else if prop.ThreadID != "" && resp == "join" && prev != "join" {
		// 成立後の追加参加はスレッドにも知らせる
		_, _ = s.ChannelMessageSend(prop.ThreadID,
			fmt.Sprintf("✋ <@%s> も参加します!", uid))
	}

	// 押した本人の操作で embed を最新化(メッセージ自体を更新)
	candidates, err := b.currentCandidates(prop)
	if err != nil {
		return err
	}
	embed, err := b.buildProposalEmbed(act, candidates, responses, prop.ThreadID)
	if err != nil {
		return err
	}
	components := proposalButtons(proposalID)
	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
	return err
}

// currentCandidates は提案の日付時点の候補メンバーを再計算する。
func (b *Bot) currentCandidates(prop store.Proposal) ([]store.Prefs, error) {
	prefsAll, err := b.store.GetAllPrefs()
	if err != nil {
		return nil, err
	}
	statuses, err := b.store.GetDailyStatuses(prop.Date)
	if err != nil {
		return nil, err
	}
	return candidatesFor(prop.ActivityID, prefsAll, statuses), nil
}

// establishProposal はスレッドを作成し、参加者・呼んでメンバーに通知する。
func (b *Bot) establishProposal(s *discordgo.Session, prop store.Proposal, act activity.Activity, responses map[string][]string) (string, error) {
	t, err := s.MessageThreadStart(prop.ChannelID, prop.MessageID,
		fmt.Sprintf("%s %s %s", b.todayLabel(), act.Emoji, act.Name), 1440)
	if err != nil {
		// 過去にスレッド作成は成功したが DB 保存に失敗していた場合、
		// 既存スレッドを拾い直して復旧する(メッセージ起点のスレッド ID は
		// 元メッセージ ID と同じ)
		if ch, cerr := s.Channel(prop.MessageID); cerr == nil && ch.IsThread() {
			t = ch
		} else {
			return "", fmt.Errorf("スレッド作成失敗: %w", err)
		}
	}
	if err := b.store.SetProposalThread(prop.ID, t.ID); err != nil {
		// スレッド自体はできているので失敗扱いにしない(次回押下時に上の復旧パスで保存し直す)
		slog.Error("thread_id の保存に失敗(次回の操作で復旧します)", "id", prop.ID, "err", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "🎉 **%s、人数が集まりました!** 出発時間や行き先はここで相談してください。\n\n", act.Name)
	fmt.Fprintf(&sb, "✋ 参加: %s\n", mentionList(responses["join"]))
	if len(responses["standby"]) > 0 {
		fmt.Fprintf(&sb, "🔔 呼んでメンバー: %s(良ければ合流どうぞ!)\n", mentionList(responses["standby"]))
	}
	if _, err := s.ChannelMessageSend(t.ID, sb.String()); err != nil {
		slog.Error("スレッドへの通知送信失敗", "err", err)
	}
	slog.Info("スレッド成立", "activity", act.ID, "thread", t.ID)
	return t.ID, nil
}
