// Package activity はアクティビティ・時間帯・スタンスのコード内マスタ定義。
// 追加・変更はこのファイルを編集するだけでよい。
package activity

// Activity はマッチング対象の活動。
type Activity struct {
	ID        string
	Name      string
	Emoji     string
	Threshold int // 「参加する」がこの人数に達したらスレッドを作成する
}

// All が 20 時投稿・設定メニューの両方の元データになる。
var All = []Activity{
	{ID: "levelre", Name: "レベルレ", Emoji: "🎲", Threshold: 4},
	{ID: "mobhunt", Name: "モブハン", Emoji: "🐉", Threshold: 2},
	{ID: "pw", Name: "PW作成", Emoji: "⚔️", Threshold: 3},
	{ID: "map", Name: "地図", Emoji: "🗺️", Threshold: 2},
}

// ByID は ID からアクティビティを引く。
func ByID(id string) (Activity, bool) {
	for _, a := range All {
		if a.ID == id {
			return a, true
		}
	}
	return Activity{}, false
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
