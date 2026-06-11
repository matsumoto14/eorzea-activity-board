// Package store は SQLite への永続化。スキーマ管理と CRUD をまとめる。
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// proposalsTable は schema と再構築マイグレーションの両方で使う。
// UNIQUE は (date, activity_id, detail): 同日同カテゴリでも種目が違えば
// 別の募集カードを立てられる(同日同種目の二重カードだけを防ぐ)。
const proposalsTable = `
CREATE TABLE IF NOT EXISTS proposals (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	date        TEXT NOT NULL,
	activity_id TEXT NOT NULL,
	detail      TEXT NOT NULL DEFAULT '',
	channel_id  TEXT NOT NULL DEFAULT '',
	message_id  TEXT NOT NULL DEFAULT '',
	thread_id   TEXT NOT NULL DEFAULT '',
	UNIQUE (date, activity_id, detail)
);`

const schema = `
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS user_prefs (
	user_id    TEXT PRIMARY KEY,
	activities TEXT NOT NULL DEFAULT '',
	timeslots  TEXT NOT NULL DEFAULT '',
	stance     TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS daily_status (
	user_id TEXT NOT NULL,
	date    TEXT NOT NULL,
	status  TEXT NOT NULL,
	PRIMARY KEY (user_id, date)
);
CREATE TABLE IF NOT EXISTS pw_progress (
	user_id    TEXT PRIMARY KEY,
	progress   TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
` + proposalsTable + `
CREATE TABLE IF NOT EXISTS responses (
	proposal_id INTEGER NOT NULL,
	user_id     TEXT NOT NULL,
	response    TEXT NOT NULL,
	PRIMARY KEY (proposal_id, user_id)
);
`

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("DBディレクトリ作成失敗: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite は単一ライターなので接続を 1 本に絞り busy エラーを避ける
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("スキーマ適用失敗: %w", err)
	}
	// 素朴なマイグレーション: detail 列が無い既存 DB に追加する
	// (新規 DB は schema で作成済みのため duplicate column になり、無視してよい)
	if _, err := db.Exec(`ALTER TABLE proposals ADD COLUMN detail TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		db.Close()
		return nil, fmt.Errorf("マイグレーション失敗(proposals.detail): %w", err)
	}
	if err := migrateProposalsUnique(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("マイグレーション失敗(proposals UNIQUE): %w", err)
	}
	return &Store{db: db}, nil
}

// migrateProposalsUnique は旧 UNIQUE(date, activity_id) のテーブルを
// UNIQUE(date, activity_id, detail) に作り直す(SQLite は制約変更不可のため再構築)。
// sqlite_master の SQL はこのパッケージが書いた schema 文字列そのものなので、
// 現行の UNIQUE 句を含むかどうかで新旧を判定できる。
func migrateProposalsUnique(db *sql.DB) error {
	var tblSQL string
	if err := db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'proposals'`).Scan(&tblSQL); err != nil {
		return err
	}
	if strings.Contains(tblSQL, "UNIQUE (date, activity_id, detail)") {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		`ALTER TABLE proposals RENAME TO proposals_old`,
		proposalsTable,
		`INSERT INTO proposals (id, date, activity_id, detail, channel_id, message_id, thread_id)
		 SELECT id, date, activity_id, detail, channel_id, message_id, thread_id FROM proposals_old`,
		`DROP TABLE proposals_old`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// joinList / splitList: カンマ区切り文字列と []string の相互変換。
// strings.Split("") は [""] を返すため空チェックが必要。
func joinList(v []string) string { return strings.Join(v, ",") }

func splitList(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

// ---- settings ----

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// GetSetting は未設定なら空文字列を返す。
func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// ---- user_prefs ----

type Prefs struct {
	UserID     string
	Activities []string
	TimeSlots  []string
	Stance     string
}

func (s *Store) GetPrefs(userID string) (Prefs, error) {
	p := Prefs{UserID: userID}
	var acts, slots string
	err := s.db.QueryRow(
		`SELECT activities, timeslots, stance FROM user_prefs WHERE user_id = ?`,
		userID).Scan(&acts, &slots, &p.Stance)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	p.Activities = splitList(acts)
	p.TimeSlots = splitList(slots)
	return p, nil
}

func (s *Store) GetAllPrefs() ([]Prefs, error) {
	rows, err := s.db.Query(`SELECT user_id, activities, timeslots, stance FROM user_prefs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Prefs
	for rows.Next() {
		var p Prefs
		var acts, slots string
		if err := rows.Scan(&p.UserID, &acts, &slots, &p.Stance); err != nil {
			return nil, err
		}
		p.Activities = splitList(acts)
		p.TimeSlots = splitList(slots)
		out = append(out, p)
	}
	return out, rows.Err()
}

// setPref は user_prefs の 1 カラムだけを upsert する。
// column は呼び出し側のリテラル固定(ユーザー入力を渡さないこと)。
func (s *Store) setPref(userID, column, value string) error {
	_, err := s.db.Exec(fmt.Sprintf(
		`INSERT INTO user_prefs (user_id, %[1]s, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET %[1]s = excluded.%[1]s, updated_at = excluded.updated_at`,
		column), userID, value, now())
	return err
}

func (s *Store) SetPrefActivities(userID string, ids []string) error {
	return s.setPref(userID, "activities", joinList(ids))
}

func (s *Store) SetPrefTimeSlots(userID string, ids []string) error {
	return s.setPref(userID, "timeslots", joinList(ids))
}

func (s *Store) SetPrefStance(userID, stance string) error {
	return s.setPref(userID, "stance", stance)
}

// ---- daily_status ----

func (s *Store) SetDailyStatus(userID, date, status string) error {
	_, err := s.db.Exec(
		`INSERT INTO daily_status (user_id, date, status) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, date) DO UPDATE SET status = excluded.status`,
		userID, date, status)
	return err
}

// GetDailyStatuses は date の全員分の状態を user_id -> status で返す。
func (s *Store) GetDailyStatuses(date string) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT user_id, status FROM daily_status WHERE date = ?`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var uid, st string
		if err := rows.Scan(&uid, &st); err != nil {
			return nil, err
		}
		out[uid] = st
	}
	return out, rows.Err()
}

// ---- pw_progress ----

type PWProgress struct {
	UserID    string
	Progress  string
	UpdatedAt string
}

func (s *Store) SetPWProgress(userID, progress string) error {
	_, err := s.db.Exec(
		`INSERT INTO pw_progress (user_id, progress, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET progress = excluded.progress, updated_at = excluded.updated_at`,
		userID, progress, now())
	return err
}

func (s *Store) GetPWProgress(userID string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT progress FROM pw_progress WHERE user_id = ?`, userID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) GetAllPWProgress() ([]PWProgress, error) {
	rows, err := s.db.Query(
		`SELECT user_id, progress, updated_at FROM pw_progress ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PWProgress
	for rows.Next() {
		var p PWProgress
		if err := rows.Scan(&p.UserID, &p.Progress, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- proposals ----

type Proposal struct {
	ID         int64
	Date       string
	ActivityID string
	Detail     string // 種目(activity.Variant の ID)。未指定は ""
	ChannelID  string
	MessageID  string
	ThreadID   string
}

// CreateProposal は提案を作成して返す。同日同活動同種目が既にあれば
// 既存の提案を created=false で返す(呼び出し側の引き直し不要)。
// 同日同活動でも種目(detail)が違えば別の提案として作成できる。
func (s *Store) CreateProposal(date, activityID, detail string) (Proposal, bool, error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO proposals (date, activity_id, detail) VALUES (?, ?, ?)`,
		date, activityID, detail)
	if err != nil {
		return Proposal{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Proposal{}, false, err
	}
	p := Proposal{Date: date, ActivityID: activityID}
	err = s.db.QueryRow(
		`SELECT id, detail, channel_id, message_id, thread_id FROM proposals
		 WHERE date = ? AND activity_id = ? AND detail = ?`,
		date, activityID, detail).Scan(&p.ID, &p.Detail, &p.ChannelID, &p.MessageID, &p.ThreadID)
	return p, n > 0, err
}

// FindProposal は同日同活動同種目の提案を探す(無ければ ok=false)。
// 種目なしカテゴリの「すでに募集中か」の判定に使う。
func (s *Store) FindProposal(date, activityID, detail string) (Proposal, bool, error) {
	p := Proposal{Date: date, ActivityID: activityID}
	err := s.db.QueryRow(
		`SELECT id, detail, channel_id, message_id, thread_id FROM proposals
		 WHERE date = ? AND activity_id = ? AND detail = ?`,
		date, activityID, detail).Scan(&p.ID, &p.Detail, &p.ChannelID, &p.MessageID, &p.ThreadID)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, false, nil
	}
	return p, err == nil, err
}

// FindProposalsByActivity は同日同活動の提案を種目問わずすべて返す。
// 種目セレクトに「募集中」の印を付けるために使う。
func (s *Store) FindProposalsByActivity(date, activityID string) ([]Proposal, error) {
	rows, err := s.db.Query(
		`SELECT id, detail, channel_id, message_id, thread_id FROM proposals
		 WHERE date = ? AND activity_id = ?`,
		date, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Proposal
	for rows.Next() {
		p := Proposal{Date: date, ActivityID: activityID}
		if err := rows.Scan(&p.ID, &p.Detail, &p.ChannelID, &p.MessageID, &p.ThreadID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteProposal はカード投稿に失敗した提案の巻き戻しに使う。回答も併せて消す。
func (s *Store) DeleteProposal(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM responses WHERE proposal_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM proposals WHERE id = ?`, id)
	return err
}

func (s *Store) SetProposalMessage(id int64, channelID, messageID string) error {
	_, err := s.db.Exec(
		`UPDATE proposals SET channel_id = ?, message_id = ? WHERE id = ?`,
		channelID, messageID, id)
	return err
}

func (s *Store) SetProposalThread(id int64, threadID string) error {
	_, err := s.db.Exec(`UPDATE proposals SET thread_id = ? WHERE id = ?`, threadID, id)
	return err
}

func (s *Store) GetProposal(id int64) (Proposal, error) {
	p := Proposal{ID: id}
	err := s.db.QueryRow(
		`SELECT date, activity_id, detail, channel_id, message_id, thread_id FROM proposals WHERE id = ?`,
		id).Scan(&p.Date, &p.ActivityID, &p.Detail, &p.ChannelID, &p.MessageID, &p.ThreadID)
	return p, err
}

// ---- responses ----

// SetResponse は以前の回答を返しつつ上書きする(未回答なら "")。
func (s *Store) SetResponse(proposalID int64, userID, response string) (string, error) {
	var prev string
	err := s.db.QueryRow(
		`SELECT response FROM responses WHERE proposal_id = ? AND user_id = ?`,
		proposalID, userID).Scan(&prev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_, err = s.db.Exec(
		`INSERT INTO responses (proposal_id, user_id, response) VALUES (?, ?, ?)
		 ON CONFLICT(proposal_id, user_id) DO UPDATE SET response = excluded.response`,
		proposalID, userID, response)
	return prev, err
}

// GetResponses は response -> 押した順の user_id 列を返す。
func (s *Store) GetResponses(proposalID int64) (map[string][]string, error) {
	rows, err := s.db.Query(
		`SELECT user_id, response FROM responses WHERE proposal_id = ? ORDER BY rowid`,
		proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var uid, resp string
		if err := rows.Scan(&uid, &resp); err != nil {
			return nil, err
		}
		out[resp] = append(out[resp], uid)
	}
	return out, rows.Err()
}
