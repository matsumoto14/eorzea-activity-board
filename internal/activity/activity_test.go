package activity

import (
	"strings"
	"testing"
)

// セレクトメニューの仕様上の上限(本体 25 件・種目は「相談して決める」+1 で 25 件)。
func TestSelectMenuLimits(t *testing.T) {
	if len(All) > 25 {
		t.Fatalf("アクティビティが %d 件あり、セレクトメニューの上限 25 件を超えている", len(All))
	}
	for _, a := range All {
		if len(a.Variants)+1 > 25 {
			t.Errorf("%s の種目が %d 件あり、「相談して決める」を足すと 25 件を超える", a.ID, len(a.Variants))
		}
	}
}

// ID は CustomID の区切り文字(:)やカンマ区切り保存と衝突しないこと。
func TestIDCharset(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range All {
		if strings.ContainsAny(a.ID, ":,") {
			t.Errorf("アクティビティ ID %q に区切り文字が含まれている", a.ID)
		}
		if seen[a.ID] {
			t.Errorf("アクティビティ ID %q が重複している", a.ID)
		}
		seen[a.ID] = true
		vseen := map[string]bool{}
		for _, v := range a.Variants {
			if strings.ContainsAny(v.ID, ":,") {
				t.Errorf("種目 ID %s/%q に区切り文字が含まれている", a.ID, v.ID)
			}
			if vseen[v.ID] {
				t.Errorf("種目 ID %s/%q が重複している", a.ID, v.ID)
			}
			vseen[v.ID] = true
		}
	}
}

func TestByIDResolvesLegacy(t *testing.T) {
	cases := map[string]string{
		"roulette":   "roulette", // 現行 ID はそのまま
		"levelre":    "roulette",
		"exroulette": "roulette",
		"alliance":   "roulette",
		"pw":         "weapon",
		"goldsaucer": "social",
	}
	for in, want := range cases {
		a, ok := ByID(in)
		if !ok || a.ID != want {
			t.Errorf("ByID(%q) = %q, %v; want %q", in, a.ID, ok, want)
		}
	}
	if _, ok := ByID("unknown"); ok {
		t.Error("不明 ID が解決されてしまった")
	}
}

// 旧 ID のエイリアスはすべて現行のアクティビティに解決できること。
func TestLegacyIDsResolve(t *testing.T) {
	for old, cur := range legacyIDs {
		if a, ok := ByID(old); !ok || a.ID != cur {
			t.Errorf("旧 ID %q が現行 %q に解決できない", old, cur)
		}
	}
}

func TestNormalizeIDs(t *testing.T) {
	got := NormalizeIDs([]string{"levelre", "exroulette", "map", "unknown", "map", "pw"})
	want := []string{"roulette", "map", "weapon"}
	if len(got) != len(want) {
		t.Fatalf("NormalizeIDs = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("NormalizeIDs[%d] = %q; want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestFreeActivity(t *testing.T) {
	a, ok := ByID("free")
	if !ok || !a.FreeText || a.ID != Free.ID {
		t.Fatalf("ByID(free) = %+v, %v", a, ok)
	}
	// detail はユーザー入力テキストとしてそのまま表示される
	if got := a.Display("絶竜詩 P6 練習"); got != "絶竜詩 P6 練習" {
		t.Errorf("Display(テキスト) = %q", got)
	}
	if got := a.Display(""); got != "フリー募集" {
		t.Errorf("Display(空) = %q", got)
	}
	// All には載せない(やりたいこと設定・サマリー集計の対象外)
	for _, x := range All {
		if x.ID == Free.ID {
			t.Error("Free が All に含まれている")
		}
	}
}

func TestDisplay(t *testing.T) {
	m, _ := ByID("map")
	if got := m.Display("g17"); got != "地図(G17)" {
		t.Errorf("Display(g17) = %q", got)
	}
	if got := m.Display(""); got != "地図" {
		t.Errorf("Display(空) = %q", got)
	}
	if got := m.Display("unknown"); got != "地図" {
		t.Errorf("Display(不明) = %q", got)
	}
}
