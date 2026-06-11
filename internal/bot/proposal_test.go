package bot

import (
	"testing"

	"eorzea-activity-board/internal/store"
)

func uids(prefs []store.Prefs) []string {
	out := make([]string, len(prefs))
	for i, p := range prefs {
		out[i] = p.UserID
	}
	return out
}

func TestCandidatesFor(t *testing.T) {
	prefsAll := []store.Prefs{
		{UserID: "u1", Activities: []string{"roulette", "weapon"}},
		{UserID: "u2", Activities: []string{"roulette"}},
		{UserID: "u3", Activities: []string{"map"}},
		{UserID: "u4", Activities: []string{"roulette"}},
		// u5 は旧 ID のまま保存されている(levelre→roulette / pw→weapon に吸収される)
		{UserID: "u5", Activities: []string{"levelre", "pw"}},
	}
	// candidatesFor は normalizePrefs 済みの prefs を前提とする(旧 ID の吸収はここ)
	prefsAll = normalizePrefs(prefsAll)

	tests := []struct {
		name     string
		actID    string
		statuses map[string]string
		want     []string
	}{
		{
			name:     "やりたい登録がある人だけ(旧 ID も吸収)",
			actID:    "roulette",
			statuses: map[string]string{},
			want:     []string{"u1", "u2", "u4", "u5"},
		},
		{
			name:     "今日は無理(no)の人は除外",
			actID:    "roulette",
			statuses: map[string]string{"u2": "no", "u5": "no"},
			want:     []string{"u1", "u4"},
		},
		{
			name:     "ok / maybe は候補に残る",
			actID:    "roulette",
			statuses: map[string]string{"u1": "ok", "u2": "maybe", "u4": "no", "u5": "no"},
			want:     []string{"u1", "u2"},
		},
		{
			name:     "該当登録者なしなら空",
			actID:    "mobhunt",
			statuses: map[string]string{},
			want:     nil,
		},
		{
			name:     "単一登録の活動",
			actID:    "map",
			statuses: map[string]string{},
			want:     []string{"u3"},
		},
		{
			name:     "旧 pw 登録は weapon の候補になる",
			actID:    "weapon",
			statuses: map[string]string{},
			want:     []string{"u1", "u5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uids(candidatesFor(tt.actID, prefsAll, tt.statuses))
			if len(got) != len(tt.want) {
				t.Fatalf("candidatesFor(%s) = %v; want %v", tt.actID, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("candidatesFor(%s)[%d] = %q; want %q (%v)", tt.actID, i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestMentionList(t *testing.T) {
	if got := mentionList(nil); got != "—" {
		t.Errorf("mentionList(nil) = %q; want —", got)
	}
	if got := mentionList([]string{"1", "2"}); got != "<@1> <@2>" {
		t.Errorf("mentionList = %q", got)
	}
}

func TestLimitEmbed(t *testing.T) {
	if got := limitEmbed("abc", 5); got != "abc" {
		t.Errorf("limitEmbed(短い) = %q", got)
	}
	if got := limitEmbed("abcde", 5); got != "abcde" {
		t.Errorf("limitEmbed(ちょうど) = %q", got)
	}
	if got := limitEmbed("abcdef", 5); got != "abcd…" {
		t.Errorf("limitEmbed(超過) = %q", got)
	}
	// マルチバイトでもルール単位で切れること(バイト境界で壊れない)
	if got := limitEmbed("あいうえおか", 5); got != "あいうえ…" {
		t.Errorf("limitEmbed(日本語) = %q", got)
	}
}

func TestProgressBar(t *testing.T) {
	cases := []struct {
		joins, threshold int
		want             string
	}{
		{0, 3, "▱▱▱"},
		{1, 3, "▰▱▱"},
		{3, 3, "▰▰▰"},
		{5, 3, "▰▰▰"}, // 超過してもバー長は threshold まで
		{1, 0, "▰"},   // threshold<1 は 1 として扱う
	}
	for _, c := range cases {
		if got := progressBar(c.joins, c.threshold); got != c.want {
			t.Errorf("progressBar(%d,%d) = %q; want %q", c.joins, c.threshold, got, c.want)
		}
	}
}
