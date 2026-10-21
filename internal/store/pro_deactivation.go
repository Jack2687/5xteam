package store

import (
	"chatgpt-space-merge/internal/model"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Save only the web session used for deletion. OAuth credentials and all
// payment/history records remain intact even when login reports a dead user.
func (s *Store) SaveProDeletionSession(email, session string) error {
	if !json.Valid([]byte(session)) {
		return errors.New("登录 Session 格式无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var raw, sealed string
	if err := s.db.QueryRow("SELECT profile,encrypted_credentials FROM mail_accounts WHERE email=?", strings.ToLower(email)).Scan(&raw, &sealed); err != nil {
		return err
	}
	var p model.MailAccountProfile
	var c model.MailAccountCredentials
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return err
	}
	if p.ManagementScope != "pro" {
		return errors.New("账号不在 Pro 管理中")
	}
	plain, err := s.decrypt(sealed)
	if err != nil {
		return err
	}
	if err = json.Unmarshal([]byte(plain), &c); err != nil {
		return err
	}
	c.ChatGPTSession = session
	p.ChatGPTSessionPresent = true
	sealed, err = s.encrypt(mustJSON(c))
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE mail_accounts SET profile=?,encrypted_credentials=? WHERE email=?", mustJSON(p), sealed, strings.ToLower(email))
	return err
}
func (s *Store) DueProDeactivations(now time.Time) ([]model.MailAccountProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT profile FROM mail_accounts WHERE json_extract(profile,'$.management_scope')='pro' AND json_extract(profile,'$.pro_deactivation.submitted_at') IS NOT NULL AND COALESCE(json_extract(profile,'$.pro_workflow_running'),0)=0 AND COALESCE(json_extract(profile,'$.pro_auto.stop_requested'),0)=0 AND COALESCE(json_extract(profile,'$.pro_migration.paused'),0)=0 AND julianday(json_extract(profile,'$.pro_deactivation.next_check_at'))<=julianday(?) ORDER BY julianday(json_extract(profile,'$.pro_deactivation.next_check_at')) LIMIT 10`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.MailAccountProfile{}
	for rows.Next() {
		var raw string
		var p model.MailAccountProfile
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
