// Package activity はアクティビティ・時間帯・スタンスのコード内マスタ定義。
// 追加・変更はこのファイルを編集するだけでよい。
package activity

// Variant はアクティビティ内の種目(例: 地図の G17、零式の練習)。
// マッチング(やりたいこと設定・候補人数)はアクティビティ単位で行い、
// 種目は募集を開くときに選んでカード・スレッド名に載せるだけの軽い情報。
type Variant struct {
	ID    string
	Label string
}

// Activity はマッチング対象の活動。
type Activity struct {
	ID          string
	Name        string
	Emoji       string
	Threshold   int // 「参加する」がこの人数に達したらスレッドを作成する
	Variants    []Variant
	FreeText    bool // true なら detail を種目 ID ではなくユーザー入力テキストとして扱う
	HasProgress bool // true なら募集カードに武器進捗メモ(pw_progress)を添える
}

// Free はカテゴリ外のフリー募集(ユーザーが入力したテキストでそのまま募集を開く)。
// All には載せない: やりたいこと設定・サマリーの候補集計の対象外で、
// 募集カード・スレッドの仕組みだけを通常募集と共有する。
var Free = Activity{ID: "free", Name: "フリー募集", Emoji: "✍️", Threshold: 2, FreeText: true}

// All が 20 時投稿・設定メニューの両方の元データになる。
// セレクトメニューの上限(25 件)を超えないこと。
// 種目も同様に「相談して決める」の 1 件を足して 25 件以内に収めること。
var All = []Activity{
	{ID: "roulette", Name: "ルーレット", Emoji: "🎲", Threshold: 2, Variants: []Variant{
		{ID: "levelre", Label: "レベルレ"},
		{ID: "expert", Label: "エキルレ"},
		{ID: "high", Label: "ハイルレ・Lv帯別ID"},
		{ID: "trial", Label: "討滅ルレ"},
		{ID: "normalraid", Label: "ノーマルレイドルレ"},
		{ID: "alliance", Label: "アラルレ"},
		{ID: "msq", Label: "メインルレ"},
		{ID: "frontline", Label: "フロントラインルレ"},
	}},
	{ID: "mobhunt", Name: "モブハント", Emoji: "🐉", Threshold: 2, Variants: []Variant{
		{ID: "dt", Label: "黄金"},
		{ID: "ew", Label: "暁月"},
		{ID: "shb", Label: "漆黒"},
		{ID: "stb", Label: "紅蓮"},
		{ID: "hw", Label: "蒼天"},
		{ID: "arr", Label: "新生"},
	}},
	{ID: "map", Name: "地図", Emoji: "🗺️", Threshold: 2, Variants: []Variant{
		{ID: "latest", Label: "最新地図"},
		{ID: "g17", Label: "G17"},
		{ID: "g16", Label: "G16"},
		{ID: "g15", Label: "G15"},
		{ID: "g14", Label: "G14"},
		{ID: "g12", Label: "G12"},
		{ID: "old", Label: "過去地図"},
	}},
	{ID: "weapon", Name: "武器作成", Emoji: "⚔️", Threshold: 2, HasProgress: true, Variants: []Variant{
		{ID: "pw", Label: "PW(ファントム)"},
		{ID: "mw", Label: "MW(マンダヴィル)"},
		{ID: "rw", Label: "RW(レジスタンス)"},
		{ID: "ew", Label: "EW(エウレカ)"},
		{ID: "aw", Label: "AW(アニマ)"},
		{ID: "zw", Label: "ZW(ゾディアック)"},
	}},
	{ID: "extreme", Name: "極討滅戦", Emoji: "🔥", Threshold: 2, Variants: []Variant{
		{ID: "latest", Label: "最新極"},
		{ID: "mount", Label: "マウント周回"},
		{ID: "whistle", Label: "笛周回"},
		{ID: "old", Label: "過去極"},
	}},
	{ID: "unreal", Name: "幻討滅戦", Emoji: "🌙", Threshold: 2},
	{ID: "savage", Name: "零式", Emoji: "🛡️", Threshold: 2, Variants: []Variant{
		{ID: "latest", Label: "最新零式"},
		{ID: "clear", Label: "消化"},
		{ID: "practice", Label: "練習"},
		{ID: "first", Label: "初見"},
		{ID: "mount", Label: "マウント周回"},
	}},
	{ID: "ultimate", Name: "絶", Emoji: "💀", Threshold: 2, Variants: []Variant{
		{ID: "ucob", Label: "絶バハムート"},
		{ID: "uwu", Label: "絶アルテマ"},
		{ID: "tea", Label: "絶アレキサンダー"},
		{ID: "dsr", Label: "絶竜詩戦争"},
		{ID: "top", Label: "絶オメガ検証戦"},
	}},
	{ID: "normalraid", Name: "ノーマルレイド", Emoji: "🗡️", Threshold: 2, Variants: []Variant{
		{ID: "latest", Label: "最新ノーマルレイド"},
		{ID: "old", Label: "過去ノーマルレイド"},
	}},
	{ID: "allianceraid", Name: "アライアンスレイド", Emoji: "🏛️", Threshold: 2, Variants: []Variant{
		{ID: "latest", Label: "最新アラ"},
		{ID: "weekly", Label: "週制限アラ"},
		{ID: "dt", Label: "黄金"},
		{ID: "ew", Label: "暁月"},
		{ID: "shb", Label: "漆黒"},
		{ID: "stb", Label: "紅蓮"},
		{ID: "hw", Label: "蒼天"},
		{ID: "arr", Label: "新生"},
	}},
	{ID: "field", Name: "特殊フィールド", Emoji: "🌋", Threshold: 2, Variants: []Variant{
		{ID: "occult", Label: "Occult Crescent"},
		{ID: "eureka", Label: "エウレカ"},
		{ID: "bozja", Label: "ボズヤ"},
	}},
	{ID: "deepdungeon", Name: "ディープダンジョン", Emoji: "🌀", Threshold: 2, Variants: []Variant{
		{ID: "potd", Label: "死者の宮殿"},
		{ID: "hoh", Label: "アメノミハシラ"},
		{ID: "eo", Label: "オルト・エウレカ"},
		{ID: "pilgrim", Label: "Pilgrim's Traverse"},
	}},
	{ID: "variant", Name: "ヴァリアント・アナザー", Emoji: "🧭", Threshold: 2, Variants: []Variant{
		{ID: "sildihn", Label: "シラディハ水道"},
		{ID: "rokkon", Label: "六根山"},
		{ID: "aloalo", Label: "アロアロ島"},
		{ID: "criterion", Label: "異聞(アナザー)"},
		{ID: "criterion_savage", Label: "異聞零式(アナザー)"},
	}},
	{ID: "bluemage", Name: "青魔道士", Emoji: "🔵", Threshold: 2, Variants: []Variant{
		{ID: "learning", Label: "ラーニング"},
		{ID: "log", Label: "青魔ログ"},
		{ID: "raid", Label: "青魔レイド"},
	}},
	{ID: "pvp", Name: "PvP", Emoji: "🚩", Threshold: 2, Variants: []Variant{
		{ID: "frontline", Label: "フロントライン"},
		{ID: "cc_casual", Label: "クリコン(カジュアル)"},
		{ID: "cc_ranked", Label: "クリコン(ランクマッチ)"},
		{ID: "rivalwings", Label: "ライバルウィングズ"},
		{ID: "series", Label: "シリーズ報酬進行"},
	}},
	{ID: "social", Name: "交流・ライト", Emoji: "🎪", Threshold: 2, Variants: []Variant{
		{ID: "ss", Label: "SS撮影"},
		{ID: "housing", Label: "ハウジング見学"},
		{ID: "fishing", Label: "釣り"},
		{ID: "goldsaucer", Label: "ゴールドソーサー"},
		{ID: "help", Label: "お手伝い"},
		{ID: "chat", Label: "雑談VC"},
	}},
	{ID: "other", Name: "その他・なんでも", Emoji: "🎮", Threshold: 2},
}

// legacyIDs は旧アクティビティ ID から現行 ID への対応。
// DB(user_prefs.activities / proposals.activity_id)に残る旧 ID を
// 読み出し時に吸収し、ユーザーの再登録なしで移行する。
var legacyIDs = map[string]string{
	"levelre":    "roulette", // 旧: レベルレ
	"exroulette": "roulette", // 旧: エキルレ
	"alliance":   "roulette", // 旧: アラルレ
	"pw":         "weapon",   // 旧: PW作成
	"goldsaucer": "social",   // 旧: ゴールドソーサー
}

// CanonicalID は旧 ID を現行 ID に解決する(現行 ID はそのまま)。
func CanonicalID(id string) string {
	if n, ok := legacyIDs[id]; ok {
		return n
	}
	return id
}

// ByID は ID からアクティビティを引く。旧 ID も現行の定義に解決される。
func ByID(id string) (Activity, bool) {
	id = CanonicalID(id)
	if id == Free.ID {
		return Free, true
	}
	for _, a := range All {
		if a.ID == id {
			return a, true
		}
	}
	return Activity{}, false
}

// NormalizeIDs は保存済みの ID 列を現行 ID に解決し、重複と不明 ID を除く。
// user_prefs の読み出し側で必ず通すこと(旧 ID の吸収はここで行う)。
func NormalizeIDs(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		c := CanonicalID(id)
		if seen[c] {
			continue
		}
		if _, ok := ByID(c); !ok {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// VariantLabel は種目 ID の表示名を返す(未指定・不明は空)。
func (a Activity) VariantLabel(id string) string {
	for _, v := range a.Variants {
		if v.ID == id {
			return v.Label
		}
	}
	return ""
}

// Display は募集カード・スレッド名用の表示名。種目があれば「地図(G17)」形式。
// フリー募集はユーザー入力テキスト(detail)をそのまま表示する。
func (a Activity) Display(detail string) string {
	if a.FreeText && detail != "" {
		return detail
	}
	if l := a.VariantLabel(detail); l != "" {
		return a.Name + "(" + l + ")"
	}
	return a.Name
}

// TimeSlot は参加しやすい時間帯。
type TimeSlot struct {
	ID    string
	Label string
}

var TimeSlots = []TimeSlot{
	{ID: "early", Label: "〜20時"},
	{ID: "prime", Label: "20〜22時"},
	{ID: "late", Label: "22〜24時"},
	{ID: "midnight", Label: "24時〜"},
	{ID: "flex", Label: "不定・日による"},
}

// Stance は参加スタンス。
type Stance struct {
	ID    string
	Label string
}

var Stances = []Stance{
	{ID: "host", Label: "主催もできる"},
	{ID: "join", Label: "誘われたら行く"},
	{ID: "fill", Label: "人数合わせならOK"},
}

func StanceLabel(id string) string {
	for _, s := range Stances {
		if s.ID == id {
			return s.Label
		}
	}
	return ""
}

func TimeSlotLabel(id string) string {
	for _, t := range TimeSlots {
		if t.ID == id {
			return t.Label
		}
	}
	return ""
}

// TimeSlotLabels は ID 列を表示用ラベル列に変換する(不明 ID は無視)。
func TimeSlotLabels(ids []string) []string {
	labels := make([]string, 0, len(ids))
	for _, id := range ids {
		if l := TimeSlotLabel(id); l != "" {
			labels = append(labels, l)
		}
	}
	return labels
}
