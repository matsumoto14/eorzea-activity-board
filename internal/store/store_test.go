package store

import (
	"database/sql"
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

	p1, created, err := st.CreateProposal("2026-06-11", "roulette", "levelre")
	if err != nil || !created || p1.ID == 0 {
		t.Fatalf("1回目 CreateProposal: %+v created=%v err=%v", p1, created, err)
	}
	if p1.Detail != "levelre" {
		t.Fatalf("Detail = %q; want levelre", p1.Detail)
	}

	// 同日同活動同種目は二重作成せず、既存の提案を created=false で返す
	st.SetProposalMessage(p1.ID, "chanX", "msgX")
	p2, created2, err := st.CreateProposal("2026-06-11", "roulette", "levelre")
	if err != nil {
		t.Fatal(err)
	}
	if created2 {
		t.Fatalf("二重作成された: %+v", p2)
	}
	if p2.ID != p1.ID || p2.ChannelID != "chanX" || p2.MessageID != "msgX" || p2.Detail != "levelre" {
		t.Fatalf("既存提案が返らない: %+v", p2)
	}

	// 同日同活動でも種目が違えば作成できる(2 つ目の募集が立てられる)
	p3, created3, err := st.CreateProposal("2026-06-11", "roulette", "expert")
	if err != nil || !created3 || p3.ID == p1.ID || p3.Detail != "expert" {
		t.Fatalf("別種目が作成できない: %+v created=%v err=%v", p3, created3, err)
	}

	// 別活動・別日は作成できる
	if _, created, _ := st.CreateProposal("2026-06-11", "map", ""); !created {
		t.Fatal("別活動が作成できない")
	}
	if _, created, _ := st.CreateProposal("2026-06-12", "roulette", ""); !created {
		t.Fatal("別日が作成できない")
	}
}

// 旧スキーマ(detail 列なし・UNIQUE(date, activity_id))の DB を Open すると
// detail 列の追加と UNIQUE(date, activity_id, detail) への再構築が行われること。
func TestOpenMigratesProposals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE proposals (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		date        TEXT NOT NULL,
		activity_id TEXT NOT NULL,
		channel_id  TEXT NOT NULL DEFAULT '',
		message_id  TEXT NOT NULL DEFAULT '',
		thread_id   TEXT NOT NULL DEFAULT '',
		UNIQUE (date, activity_id)
	); INSERT INTO proposals (date, activity_id, message_id) VALUES ('2026-06-10', 'pw', 'msg1');`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	st, err := Open(path)
	if err != nil {
		t.Fatalf("旧スキーマの Open に失敗: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	// 既存行は detail = '' のまま保持される(ID・メッセージも維持)
	old, found, err := st.FindProposal("2026-06-10", "pw", "")
	if err != nil || !found || old.Detail != "" || old.MessageID != "msg1" {
		t.Fatalf("旧行の読み出し: %+v found=%v err=%v", old, found, err)
	}

	// 再構築後は同日同活動でも種目が違えば作成できる(旧 UNIQUE では不可)
	p1, created, err := st.CreateProposal("2026-06-11", "roulette", "levelre")
	if err != nil || !created {
		t.Fatalf("マイグレーション後の作成: %+v created=%v err=%v", p1, created, err)
	}
	p2, created, err := st.CreateProposal("2026-06-11", "roulette", "expert")
	if err != nil || !created || p2.ID == p1.ID {
		t.Fatalf("別種目の作成: %+v created=%v err=%v", p2, created, err)
	}

	// 再 Open しても再構築は走らず冪等(行も消えない)
	st.Close()
	st2, err := Open(path)
	if err != nil {
		t.Fatalf("2 回目の Open に失敗: %v", err)
	}
	t.Cleanup(func() { st2.Close() })
	if _, found, _ := st2.FindProposal("2026-06-10", "pw", ""); !found {
		t.Fatal("再 Open 後に旧行が消えた")
	}
	if got, _ := st2.FindProposalsByActivity("2026-06-11", "roulette"); len(got) != 2 {
		t.Fatalf("再 Open 後の行数 = %d; want 2", len(got))
	}
}

func TestFindProposal(t *testing.T) {
	st := newTestStore(t)

	if _, found, err := st.FindProposal("2026-06-11", "map", "g17"); err != nil || found {
		t.Fatalf("未作成なのに found=%v err=%v", found, err)
	}

	p, _, _ := st.CreateProposal("2026-06-11", "map", "g17")
	st.SetProposalMessage(p.ID, "chan1", "msg1")

	got, found, err := st.FindProposal("2026-06-11", "map", "g17")
	if err != nil || !found {
		t.Fatalf("FindProposal: found=%v err=%v", found, err)
	}
	if got.ID != p.ID || got.Detail != "g17" || got.MessageID != "msg1" {
		t.Fatalf("FindProposal = %+v", got)
	}
	// 種目違いはヒットしない
	if _, found, _ := st.FindProposal("2026-06-11", "map", ""); found {
		t.Fatal("種目違いの提案がヒットした")
	}
}

func TestFindProposalsByActivity(t *testing.T) {
	st := newTestStore(t)
	p1, _, _ := st.CreateProposal("2026-06-11", "roulette", "levelre")
	st.SetProposalMessage(p1.ID, "chan1", "msg1")
	st.CreateProposal("2026-06-11", "roulette", "expert")
	st.CreateProposal("2026-06-11", "map", "g17")   // 別活動
	st.CreateProposal("2026-06-12", "roulette", "") // 別日

	got, err := st.FindProposalsByActivity("2026-06-11", "roulette")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d; want 2 (%+v)", len(got), got)
	}
	details := map[string]string{}
	for _, p := range got {
		details[p.Detail] = p.MessageID
	}
	if details["levelre"] != "msg1" || details["expert"] != "" {
		t.Fatalf("details = %v", details)
	}
}

func TestProposalMessageAndThread(t *testing.T) {
	st := newTestStore(t)
	p, _, _ := st.CreateProposal("2026-06-11", "weapon", "")
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
	if prop.Date != "2026-06-11" || prop.ActivityID != "weapon" ||
		prop.ChannelID != "chan1" || prop.MessageID != "msg1" || prop.ThreadID != "thread1" {
		t.Fatalf("GetProposal = %+v", prop)
	}
}

func TestDeleteProposal(t *testing.T) {
	st := newTestStore(t)
	p, _, _ := st.CreateProposal("2026-06-11", "roulette", "")
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
	if _, created, _ := st.CreateProposal("2026-06-11", "roulette", ""); !created {
		t.Fatal("削除後に再作成できない")
	}
}

func TestResponsesPrevAndOrder(t *testing.T) {
	st := newTestStore(t)
	p, _, _ := st.CreateProposal("2026-06-11", "mobhunt", "")
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
	p, _, _ := st.CreateProposal("2026-06-11", "roulette", "")
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
