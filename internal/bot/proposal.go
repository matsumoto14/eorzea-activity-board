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

// サマリーで「有力候補」として強調表示する最低人数(やりたい登録があるメンバー数)。
// メニュー自体には全活動を載せるため、この人数に満たなくても募集は開ける。
const minCandidates = 2

// settings テーブルのキー: サマリーを投稿済みの日付。
// 20 時の自動投稿が手動 !eab post と同日に重複しないようにするためのもの。
const settingSummaryDate = "summary_date"

// PostDailyProposals は今日の候補を掲示板チャンネルへ投稿する(スケジューラ用)。
// 同日に投稿済み(手動 post 含む)ならスキップする。
func (b *Bot) PostDailyProposals() {
	if _, err := b.postDailyProposals(false); err != nil {
		slog.Error("20時投稿に失敗", "err", err)
	}
}

// postDailyProposals はサマリーを投稿する。force=false(スケジューラ)は
// 同日 2 回目以降をスキップして posted=false を返す。force=true(!eab post)は
// 管理者の明示操作なので常に投稿する。
func (b *Bot) postDailyProposals(force bool) (bool, error) {
	channelID, err := b.store.GetSetting(settingBoardChannel)
	if err != nil {
		return false, err
	}
	if channelID == "" {
		slog.Warn("掲示板チャンネル未設定のため投稿をスキップ(!eab setup を実行してください)")
		return false, nil
	}

	date := b.today()
	if !force {
		if last, err := b.store.GetSetting(settingSummaryDate); err != nil {
			return false, err
		} else if last == date {
			slog.Info("今日のサマリーは投稿済みのためスキップ", "date", date)
			return false, nil
		}
	}

	prefsAll, err := b.store.GetAllPrefs()
	if err != nil {
		return false, err
	}
	prefsAll = normalizePrefs(prefsAll)
	statuses, err := b.store.GetDailyStatuses(date)
	if err != nil {
		return false, err
	}

	// 全活動の候補数を集計してサマリーに載せる。候補が少ない活動も
	// メニューから募集を開けるようにし、「候補なし=何もできない」を作らない。
	// 募集カードはここでは作らず、誰かがサマリーで選んだ時に遅延生成する。
	summary := make([]summaryItem, 0, len(activity.All))
	for _, act := range activity.All {
		candidates := candidatesFor(act.ID, prefsAll, statuses)
		summary = append(summary, summaryItem{Activity: act, Count: len(candidates)})
	}

	if err := b.postSummary(channelID, date, summary); err != nil {
		return false, err
	}
	slog.Info("活動サマリーを投稿", "date", date)
	return true, b.store.SetSetting(settingSummaryDate, date)
}

// summaryItem はサマリー1行ぶん(活動と候補人数)。
type summaryItem struct {
	Activity activity.Activity
	Count    int
}

// postSummary は「今日の活動サマリー」を 1 通だけ投稿する。
// セレクトメニューには全活動を載せ(候補が少なくても募集を開ける)、
// やりたい人が minCandidates 以上の活動だけ本文で強調する。
// CustomID に日付を埋め込み、過去のサマリーからの操作を無効化できるようにする。
func (b *Bot) postSummary(channelID, date string, items []summaryItem) error {
	// やりたい人が多い活動をメニューの上位に出す(同数は定義順)
	slices.SortStableFunc(items, func(a, b summaryItem) int { return b.Count - a.Count })

	var hot strings.Builder
	opts := make([]discordgo.SelectMenuOption, 0, len(items))
	for _, it := range items {
		if it.Count >= minCandidates {
			// 1 活動 1 行 + 空行で、項目が増えても読みやすく保つ
			fmt.Fprintf(&hot, "%s **%s** … やりたい人 **%d名**\n\n", it.Activity.Emoji, it.Activity.Name, it.Count)
		}
		opts = append(opts, discordgo.SelectMenuOption{
			Label:       fmt.Sprintf("%s %s(やりたい人 %d名)", it.Activity.Emoji, it.Activity.Name, it.Count),
			Value:       it.Activity.ID,
			Description: fmt.Sprintf("%d名で成立・スレッド自動作成", it.Activity.Threshold),
		})
	}

	var title string
	var desc strings.Builder
	desc.WriteString("ごきげんよう、諸君! 本日の調査結果をご報告いたします。\n")
	desc.WriteString("下のメニューから選べば募集開始。押すだけ・書き込み不要であります。\n\n")
	if hot.Len() > 0 {
		title = fmt.Sprintf("🎩 事件です! %s の活動候補が出揃いました", b.todayLabel())
		desc.WriteString(hot.String())
	} else {
		title = fmt.Sprintf("🎩 %s の活動ボード ── 静かな夜であります", b.todayLabel())
		desc.WriteString("ふむ、本日はまだ有力な手がかりなし……。\nですが諸君、メニューから選べば募集はいつでも始められるのであります!\n")
	}
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: desc.String(),
		Color:       0xD4AF37,
		Footer:      &discordgo.MessageEmbedFooter{Text: "選ぶと募集カードが立ちます ・ 何度でも選び直し可"},
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
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "✍️ フリー募集(自由入力)", Style: discordgo.SecondaryButton, CustomID: "free:open"},
			}},
		},
	})
	return err
}

// variantAny は種目セレクトの「相談して決める」(種目を確定しない)の値。
const variantAny = "any"

// handleFreeOpen はフリー募集の入力モーダルを開く。
// 日付は送信時の「今日」を使うため CustomID に日付は持たない
// (過去のサマリーや入口メッセージのボタンからでも常に今日の募集になる)。
func (b *Bot) handleFreeOpen(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "free:modal",
			Title:    "フリー募集 ── 好きな内容で募る",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    "free:text",
						Label:       "やりたいこと",
						Style:       discordgo.TextInputShort,
						Placeholder: "例: 絶竜詩 P6 練習 / ニーア全部 / FATE金策",
						Required:    true,
						MaxLength:   60,
					},
				}},
			},
		},
	})
}

// handleFreeModalSubmit は入力されたテキストでフリー募集カードを立てる。
func (b *Bot) handleFreeModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	text := strings.TrimSpace(modalTextInput(i.ModalSubmitData(), "free:text"))
	if text == "" {
		return respondEphemeral(s, i,
			"✍️ ふむ、内容が空のようであります。やりたいことをお書きください。", nil, nil)
	}

	// カード投稿(Discord API)が遅れても interaction を失敗させないよう先に ack する
	if err := ackEphemeral(s, i); err != nil {
		return err
	}
	return b.openProposalCard(s, i, b.today(), activity.Free, text)
}

// handleSummaryOpen はサマリーで選ばれた活動の募集を開く。
// customID は "summary:open:<日付>"。種目があるカテゴリは先に種目セレクトを
// エフェメラルで 1 ステップ挟み、無いカテゴリは即カードを立てる。
func (b *Bot) handleSummaryOpen(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) error {
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return ackUpdate(s, i)
	}
	act, ok := activity.ByID(values[0])
	if !ok {
		return fmt.Errorf("不明なアクティビティ: %s", values[0])
	}

	// 過去のサマリー(別の日付)からの操作は、当日の候補状況と無関係な
	// カードが立ってしまうため受け付けない
	parts := strings.Split(customID, ":")
	if len(parts) != 3 || parts[2] != b.today() {
		return respondEphemeral(s, i,
			"🕰️ おっと、これは過去のサマリーであります。本日のサマリー(毎晩 20 時ごろ投稿)からお選びください。", nil, nil)
	}
	date := parts[2]

	// 種目があるカテゴリは「どれをやる?」を 1 ステップだけ挟む。
	// 募集は種目ごとに開けるため、既存募集があってもセレクトを出し、
	// 募集中の種目には印を付けて合流先がわかるようにする
	if len(act.Variants) > 0 {
		return b.respondVariantSelect(s, i, date, act)
	}

	// 種目なしカテゴリですでに募集中なら、既存カードへ即案内する
	if prop, found, err := b.store.FindProposal(date, act.ID, ""); err != nil {
		return err
	} else if found && prop.MessageID != "" {
		link := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", i.GuildID, prop.ChannelID, prop.MessageID)
		return respondEphemeral(s, i,
			fmt.Sprintf("%s **%s** はすでに募集中であります → %s", act.Emoji, act.Name, link), nil, nil)
	}

	// 種目なし → そのまま募集カードを立てる。
	// カード投稿(Discord API)が遅れても interaction を失敗させないよう先に ack する
	if err := ackEphemeral(s, i); err != nil {
		return err
	}
	return b.openProposalCard(s, i, date, act, "")
}

// respondVariantSelect は種目セレクトをエフェメラルで返す。
// 「相談して決める」を先頭に置き、種目の確定を強制しない。
// すでに募集中の種目には印を付ける(選ぶと既存カードへ案内される)。
func (b *Bot) respondVariantSelect(s *discordgo.Session, i *discordgo.InteractionCreate, date string, act activity.Activity) error {
	props, err := b.store.FindProposalsByActivity(date, act.ID)
	if err != nil {
		return err
	}
	open := map[string]bool{}
	for _, p := range props {
		if p.MessageID != "" {
			open[p.Detail] = true
		}
	}
	markOpen := func(o discordgo.SelectMenuOption, detail string) discordgo.SelectMenuOption {
		if open[detail] {
			o.Emoji = &discordgo.ComponentEmoji{Name: "📣"}
			o.Description = "募集中 ─ 選ぶと既存カードへご案内"
		}
		return o
	}

	opts := make([]discordgo.SelectMenuOption, 0, len(act.Variants)+1)
	opts = append(opts, markOpen(discordgo.SelectMenuOption{
		Label:       "🤝 内容は相談して決める",
		Value:       variantAny,
		Description: "とりあえず募集を開き、詳細はカード・スレッドで",
	}, ""))
	for _, v := range act.Variants {
		opts = append(opts, markOpen(discordgo.SelectMenuOption{Label: v.Label, Value: v.ID}, v.ID))
	}
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				MenuType:    discordgo.StringSelectMenu,
				CustomID:    "summary:variant:" + date + ":" + act.ID,
				Placeholder: "▼ 種目を選ぶと募集カードが立ちます",
				Options:     opts,
			},
		}},
	}
	return respondEphemeral(s, i,
		fmt.Sprintf("%s **%s** とお見受けした! して、狙いはどれですかな?", act.Emoji, act.Name),
		components, nil)
}

// handleSummaryVariant は種目セレクトの選択で募集カードを立てる。
// customID は "summary:variant:<日付>:<活動ID>"。
func (b *Bot) handleSummaryVariant(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) error {
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return ackUpdate(s, i)
	}
	parts := strings.Split(customID, ":")
	if len(parts) != 4 {
		return fmt.Errorf("不正な CustomID: %s", customID)
	}
	date, activityID := parts[2], parts[3]
	act, ok := activity.ByID(activityID)
	if !ok {
		return fmt.Errorf("不明なアクティビティ: %s", activityID)
	}
	// 日付を跨いで放置されたセレクトからの操作は受け付けない
	if date != b.today() {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "🕰️ おっと、日付が変わってしまったようであります。本日のサマリーからお選び直しください。",
				Components: []discordgo.MessageComponent{},
			},
		})
	}
	detail := values[0]
	if detail == variantAny {
		detail = ""
	}
	if detail != "" && act.VariantLabel(detail) == "" {
		return fmt.Errorf("不明な種目: %s(%s)", detail, act.ID)
	}

	// カード投稿が遅れても 3 秒期限を破らないよう、先にこのエフェメラル自身を ack し、
	// 結果はセレクトを畳んだ確認メッセージとして同じエフェメラルに反映する
	if err := ackUpdate(s, i); err != nil {
		return err
	}
	return b.openProposalCard(s, i, date, act, detail)
}

// openProposalCard は募集カードを立てる(deferred ack 済みであることが前提)。
// 初めて選ばれた活動なら新規にカードを投稿し、既に募集中なら既存カードへ案内する。
func (b *Bot) openProposalCard(s *discordgo.Session, i *discordgo.InteractionCreate, date string, act activity.Activity, detail string) error {
	editReply := func(content string) error {
		// 種目セレクト経由の場合はセレクトを畳んで結果だけ残す
		_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &content, Components: &[]discordgo.MessageComponent{},
		})
		return err
	}

	channelID, err := b.store.GetSetting(settingBoardChannel)
	if err != nil {
		return err
	}

	// 同時選択でカードが二重投稿されないよう直列化する
	b.propMu.Lock()
	defer b.propMu.Unlock()

	prop, created, err := b.store.CreateProposal(date, act.ID, detail)
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
		msgID, err := b.postProposal(channelID, prop.ID, act, prop.Detail, candidates, interactionUser(i).ID)
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
		slog.Info("募集カードを作成", "activity", act.ID, "detail", prop.Detail, "by", interactionUser(i).ID)
		return editReply(fmt.Sprintf("%s **%s** の募集、開幕であります! カードの「✋ 参加する」を押したまえ。", act.Emoji, act.Display(prop.Detail)))
	}

	// すでに募集中 → 既存カードへのリンクを案内する
	link := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", i.GuildID, prop.ChannelID, prop.MessageID)
	return editReply(fmt.Sprintf("%s **%s** はすでに募集中であります → %s", act.Emoji, act.Display(prop.Detail), link))
}

// normalizePrefs は保存済みの旧アクティビティ ID を現行 ID に解決する。
// GetAllPrefs の直後に一度だけ通し、candidatesFor では解決済みとして扱う
// (活動ごとの候補集計で毎回正規化し直さないため)。
func normalizePrefs(prefsAll []store.Prefs) []store.Prefs {
	for n := range prefsAll {
		prefsAll[n].Activities = activity.NormalizeIDs(prefsAll[n].Activities)
	}
	return prefsAll
}

// candidatesFor は act をやりたい登録していて、今日「無理」でないメンバーを返す。
// prefsAll は normalizePrefs で旧アクティビティ ID を解決済みであること。
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
// やりたい登録があるメンバー(募集を開いた本人を除く)には本文でメンション通知する。
// embed 内のメンションは通知が飛ばないため、必ず Content に載せること。
func (b *Bot) postProposal(channelID string, proposalID int64, act activity.Activity, detail string, candidates []store.Prefs, openerID string) (string, error) {
	embed, err := b.buildProposalEmbed(act, detail, candidates, map[string][]string{}, "")
	if err != nil {
		return "", err
	}
	var pings []string
	for _, p := range candidates {
		if p.UserID != openerID {
			pings = append(pings, p.UserID)
		}
	}
	content := ""
	if len(pings) > 0 {
		content = fmt.Sprintf("📣 %s ── 諸君のやりたい %s **%s**、募集開始であります!",
			mentionList(pings), act.Emoji, act.Display(detail))
	}
	msg, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:    content,
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
// 参加状況が変わるたびに呼び直して最新化する。detail は種目 ID(未指定は "")。
func (b *Bot) buildProposalEmbed(act activity.Activity, detail string, candidates []store.Prefs, responses map[string][]string, threadID string) (*discordgo.MessageEmbed, error) {
	joins := responses["join"]
	standbys := responses["standby"]
	nos := responses["no"]
	display := act.Display(detail)

	var title, desc string
	color := 0x3498DB
	if threadID != "" {
		title = fmt.Sprintf("%s %s ── 🎉 事件解決!", act.Emoji, display)
		desc = fmt.Sprintf("お見事、諸君! 人数が揃いました。\n\n作戦会議はこちら → <#%s>", threadID)
		color = 0x2ECC71
	} else {
		title = fmt.Sprintf("%s %s ── 本日の参加者、求む!", act.Emoji, display)
		remaining := max(act.Threshold-len(joins), 0)
		desc = fmt.Sprintf("✋ 参加 **%d / %d名**  %s\n\nあと **%d名** 集まれば、自動でスレッドが立ちます 🚀",
			len(joins), act.Threshold, progressBar(len(joins), act.Threshold), remaining)
	}

	// 候補メンバーは時間帯・スタンス付きで表示し、相談の材料にする。
	// 1 人 2 行(名前 + 補足)で詰まりを避ける
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
			fmt.Fprintf(&interested, "<@%s>\n　└ %s\n", p.UserID, strings.Join(info, " / "))
		} else {
			fmt.Fprintf(&interested, "<@%s>\n", p.UserID)
		}
	}
	interestedVal := "—"
	if interested.Len() > 0 {
		interestedVal = interested.String()
	}

	// embed field value は 1024 文字が上限。超えると押下のたびに更新が失敗するためキャップする。
	// inline は 2 列まで(3 列はモバイルで潰れる)。パス・候補は全幅で余裕を持たせる
	const fieldLimit = 1024
	fields := []*discordgo.MessageEmbedField{
		{Name: "✋ 参加中", Value: limitEmbed(mentionList(joins), fieldLimit), Inline: true},
		{Name: "🔔 呼んで", Value: limitEmbed(mentionList(standbys), fieldLimit), Inline: true},
		{Name: "😴 今日はパス", Value: limitEmbed(mentionList(nos), fieldLimit)},
	}
	// フリー募集はやりたいこと設定と紐付かないため「ふだんやりたい面々」を出さない
	if !act.FreeText {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: "💭 ふだんやりたい面々", Value: limitEmbed(interestedVal, fieldLimit),
		})
	}

	// 武器作成(HasProgress)は進捗メモも相談材料として添える
	if act.HasProgress {
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
				fmt.Fprintf(&sb, "<@%s>\n　└ %s\n", p.UserID, prog)
			}
		}
		if sb.Len() > 0 {
			fields = append(fields, &discordgo.MessageEmbedField{
				Name: "⚔️ 武器進捗", Value: limitEmbed(sb.String(), fieldLimit),
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
			fmt.Sprintf("✋ <@%s> 殿も参戦であります!", uid))
	}

	// 押した本人の操作で embed を最新化(メッセージ自体を更新)
	candidates, err := b.currentCandidates(prop)
	if err != nil {
		return err
	}
	embed, err := b.buildProposalEmbed(act, prop.Detail, candidates, responses, prop.ThreadID)
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
	return candidatesFor(prop.ActivityID, normalizePrefs(prefsAll), statuses), nil
}

// establishProposal はスレッドを作成し、参加者・呼んでメンバーに通知する。
func (b *Bot) establishProposal(s *discordgo.Session, prop store.Proposal, act activity.Activity, responses map[string][]string) (string, error) {
	t, err := s.MessageThreadStart(prop.ChannelID, prop.MessageID,
		fmt.Sprintf("%s %s %s", b.todayLabel(), act.Emoji, act.Display(prop.Detail)), 1440)
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
	fmt.Fprintf(&sb, "🎉 **諸君、お集まりいただき感謝する!** %s %s、見事に成立であります。\n出発時間や行き先は、ここで相談したまえ。\n\n", act.Emoji, act.Display(prop.Detail))
	fmt.Fprintf(&sb, "✋ 参加: %s\n", mentionList(responses["join"]))
	if len(responses["standby"]) > 0 {
		fmt.Fprintf(&sb, "🔔 呼んでメンバー: %s(良ければ合流したまえ!)\n", mentionList(responses["standby"]))
	}
	if _, err := s.ChannelMessageSend(t.ID, sb.String()); err != nil {
		slog.Error("スレッドへの通知送信失敗", "err", err)
	}
	slog.Info("スレッド成立", "activity", act.ID, "thread", t.ID)
	return t.ID, nil
}
