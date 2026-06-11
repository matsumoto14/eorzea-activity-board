package store

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestSettingsRoundTrip(t *testing.T) {
	st := newTestStore(t)

	// 未設定キーは空文字列
	if v, err := st.GetSetting("nope"); err != nil || v != "" {
		t.Fatalf("GetSetting(未設定) = %q, %v; want \"\", nil", v, err)
	}

	if err := st.SetSetting("board_channel", "123"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if v, _ := st.GetSetting("board_channel"); v != "123" {
		t.Fatalf("GetSetting = %q; want 123", v)
	}
	// 上書き(ON CONFLICT)
	if err := st.SetSetting("board_channel", "456"); err != nil {
		t.Fatalf("SetSetting upsert: %v", err)
	}
	if v, _ := st.GetSetting("board_channel"); v != "456" {
		t.Fatalf("GetSetting after upsert = %q; want 456", v)
	}
}

func TestPrefsPartialUpdate(t *testing.T) {
	st := newTestStore(t)

	// 未登録ユーザーは空 Prefs
	p, err := st.GetPrefs("u1")
	if err != nil {
		t.Fatalf("GetPrefs: %v", err)
	}
	if len(p.Activities) != 0 || len(p.TimeSlots) != 0 || p.Stance != "" {
		t.Fatalf("空ユーザーが空でない: %+v", p)
	}

	// 各カラムを別々に upsert しても他カラムが消えないこと
	if err := st.SetPrefActivities("u1", []string{"levelre", "pw"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPrefTimeSlots("u1", []string{"prime"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPrefStance("u1", "host"); err != nil {
		t.Fatal(err)
	}

	p, _ = st.GetPrefs("u1")
	if len(p.Activities) != 2 || p.Activities[0] != "levelre" || p.Activities[1] != "pw" {
		t.Fatalf("Activities = %v", p.Activities)
	}
	if len(p.TimeSlots) != 1 || p.TimeSlots[0] != "prime" {
		t.Fatalf("TimeSlots = %v", p.TimeSlots)
	}
	if p.Stance != "host" {
		t.Fatalf("Stance = %q", p.Stance)
	}

	// 空リスト保存でクリアできること(空チェックで nil になる)
	if err := st.SetPrefActivities("u1", nil); err != nil {
		t.Fatal(err)
	}
	p, _ = st.GetPrefs("u1")
	if len(p.Activities) != 0 {
		t.Fatalf("クリア後 Activities = %v", p.Activities)
	}
	if p.Stance != "host" {
		t.Fatalf("クリア後に Stance が消えた = %q", p.Stance)
	}
}

func TestGetAllPrefs(t *testing.T) {
	st := newTestStore(t)
	st.SetPrefActivities("u1", []string{"levelre"})
	st.SetPrefActivities("u2", []string{"map"})

	all, err := st.GetAllPrefs()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("GetAllPrefs len = %d; want 2", len(all))
	}
}

func TestDailyStatus(t *testing.T) {
	st := newTestStore(t)
	st.SetDailyStatus("u1", "2026-06-11", "ok")
	st.SetDailyStatus("u2", "2026-06-11", "no")
	st.SetDailyStatus("u1", "2026-06-12", "maybe") // 別日

	got, err := st.GetDailyStatuses("2026-06-11")
	if err != nil {
		t.Fatal(err)
	}
	if got["u1"] != "ok" || got["u2"] != "no" {
		t.Fatalf("statuses = %v", got)
	}
	if _, ok := got["u1の別日が混入"]; ok {
		t.Fatal("別日のデータが混入")
	}
	if len(got) != 2 {
		t.Fatalf("len = %d; want 2", len(got))
	}

	// 上書き
	st.SetDailyStatus("u1", "2026-06-11", "no")
	got, _ = st.GetDailyStatuses("2026-06-11")
	if got["u1"] != "no" {
		t.Fatalf("上書き後 u1 = %q; want no", got["u1"])
	}
}

func TestPWProgress(t *testing.T) {
	st := newTestStore(t)
	if v, _ := st.GetPWProgress("u1"); v != "" {
		t.Fatalf("未登録 PWProgress = %q", v)
	}
	st.SetPWProgress("u1", "2本目")
	if v, _ := st.GetPWProgress("u1"); v != "2本目" {
		t.Fatalf("PWProgress = %q", v)
	}
	st.SetPWProgress("u1", "完成")
	if v, _ := st.GetPWProgress("u1"); v != "完成" {
		t.Fatalf("上書き後 = %q", v)
	}

	list, err := st.GetAllPWProgress()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Progress != "完成" {
		t.Fatalf("GetAllPWProgress = %+v", list)
	}
}

func TestCreateProposalIdempotent(t *testing.T) {
	st := newTestStore(t)

	p1, created, err := st.CreateProposal("2026-06-11", "levelre")
	if err != nil || !created || p1.ID == 0 {
		t.Fatalf("1回目 CreateProposal: %+v created=%v err=%v", p1, created, err)
	}

	// 同日同活動は二重作成せず、既存の提案を created=false で返す
	st.SetProposalMessage(p1.ID, "chanX", "msgX")
	p2, created2, err := st.CreateProposal("2026-06-11", "levelre")
	if err != nil {
		t.Fatal(err)
	}
	if created2 {
		t.Fatalf("二重作成された: %+v", p2)
	}
	if p2.ID != p1.ID || p2.ChannelID != "chanX" || p2.MessageID != "msgX" {
		t.Fatalf("既存提案が返らない: %+v", p2)
	}

	// 別活動・別日は作成できる
	if _, created, _ := st.CreateProposal("2026-06-11", "map"); !created {
		t.Fatal("別活動が作成できない")
	}
	if _, created, _ := st.CreateProposal("2026-06-12", "levelre"); !created {
		t.Fatal("別日が作成できない")
	}
}

func TestProposalMessageAndThread(t *testing.T) {
	st := newTestStore(t)
	p, _, _ := st.CreateProposal("2026-06-11", "pw")
	id := p.ID

	if err := st.SetProposalMessage(id, "chan1", "msg1"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetProposalThread(id, "thread1"); err != nil {
		t.Fatal(err)
	}

	prop, err := st.GetProposal(id)
	if err != nil {
		t.Fatal(err)
	}
	if prop.Date != "2026-06-11" || prop.ActivityID != "pw" ||
		prop.ChannelID != "chan1" || prop.MessageID != "msg1" || prop.ThreadID != "thread1" {
		t.Fatalf("GetProposal = %+v", prop)
	}
}

func TestDeleteProposal(t *testing.T) {
	st := newTestStore(t)
	p, _, _ := st.CreateProposal("2026-06-11", "levelre")
	st.SetResponse(p.ID, "u1", "join")

	if err := st.DeleteProposal(p.ID); err != nil {
		t.Fatal(err)
	}
	// 行が消えて、同日同活動を作り直せること(巻き戻しの要件)
	if _, err := st.GetProposal(p.ID); err == nil {
		t.Fatal("削除後も GetProposal が成功した")
	}
	got, _ := st.GetResponses(p.ID)
	if len(got) != 0 {
		t.Fatalf("回答が残っている: %v", got)
	}
	if _, created, _ := st.CreateProposal("2026-06-11", "levelre"); !created {
		t.Fatal("削除後に再作成できない")
	}
}

func TestResponsesPrevAndOrder(t *testing.T) {
	st := newTestStore(t)
	p, _, _ := st.CreateProposal("2026-06-11", "mobhunt")
	id := p.ID

	// 初回は prev が空
	prev, err := st.SetResponse(id, "u1", "join")
	if err != nil {
		t.Fatal(err)
	}
	if prev != "" {
		t.Fatalf("初回 prev = %q; want \"\"", prev)
	}

	// 押し直すと前回値が返る
	prev, _ = st.SetResponse(id, "u1", "no")
	if prev != "join" {
		t.Fatalf("押し直し prev = %q; want join", prev)
	}

	st.SetResponse(id, "u2", "join")
	st.SetResponse(id, "u3", "standby")

	got, err := st.GetResponses(id)
	if err != nil {
		t.Fatal(err)
	}
	// u1 は no に変わっている
	if len(got["no"]) != 1 || got["no"][0] != "u1" {
		t.Fatalf("no = %v", got["no"])
	}
	if len(got["join"]) != 1 || got["join"][0] != "u2" {
		t.Fatalf("join = %v", got["join"])
	}
	if len(got["standby"]) != 1 || got["standby"][0] != "u3" {
		t.Fatalf("standby = %v", got["standby"])
	}
}

func TestResponsesInsertionOrder(t *testing.T) {
	st := newTestStore(t)
	p, _, _ := st.CreateProposal("2026-06-11", "levelre")
	id := p.ID
	// 押した順(rowid 順)が維持されること
	for _, u := range []string{"a", "b", "c", "d"} {
		st.SetResponse(id, u, "join")
	}
	got, _ := st.GetResponses(id)
	want := []string{"a", "b", "c", "d"}
	if len(got["join"]) != 4 {
		t.Fatalf("join len = %d", len(got["join"]))
	}
	for i, u := range want {
		if got["join"][i] != u {
			t.Fatalf("順序 [%d] = %q; want %q (%v)", i, got["join"][i], u, got["join"])
		}
	}
}
