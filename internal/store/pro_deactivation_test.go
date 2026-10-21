package store

import (
	"fmt"
	"testing"
	"time"

	"chatgpt-space-merge/internal/gptpay"
	"chatgpt-space-merge/internal/model"
)

func TestProDeletionDueRespectsPauseAndCompletion(t *testing.T) {
	s := transferStore(t)
	now := time.Now()
	past := now.Add(-time.Minute)
	for _, state := range []string{"due", "stopped", "paused", "running", "complete", "future"} {
		email := state + "@example.com"
		if _, err := s.SaveMailAccount(model.MailAccountProfile{Email: email}, model.MailAccountCredentials{Email: email}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.UpdateMailAccountManagementScope(email, "pro"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.UpdateProAccount(email, func(p *model.MailAccountProfile) {
			p.ProDeactivation = &model.ProDeactivation{Status: "queued", SubmittedAt: &past, AcceptedAt: &past, NextCheckAt: &past}
			switch state {
			case "stopped":
				p.ProAuto.StopRequested = true
			case "paused":
				p.ProMigration = &model.ProMigrationState{Paused: true}
			case "running":
				p.ProWorkflowRunning = true
			case "complete":
				p.ProDeactivation.ConfirmedAt = &past
				p.ProDeactivation.NextCheckAt = nil
			case "future":
				future := now.Add(time.Hour)
				p.ProDeactivation.NextCheckAt = &future
			}
		}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := s.DueProDeactivations(now)
	if err != nil || len(items) != 1 || items[0].Email != "due@example.com" {
		t.Fatalf("incorrect deletion scheduling: %v %v", items, err)
	}
}

func TestProDeletionIntervalRescheduleAndRecovery(t *testing.T) {
	s := transferStore(t)
	cfg, _, _, err := s.ProSettings()
	if err != nil || cfg.DeletionCheckIntervalSeconds != 300 {
		t.Fatal("missing default interval", err)
	}
	past := time.Now().Add(-time.Hour)
	states := []string{"pending", "stopped", "paused", "running", "confirmed", "cleanup"}
	for _, state := range states {
		email := state + "@example.com"
		if _, err = s.SaveMailAccount(model.MailAccountProfile{Email: email}, model.MailAccountCredentials{Email: email}); err != nil {
			t.Fatal(err)
		}
		if _, err = s.UpdateMailAccountManagementScope(email, "pro"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.UpdateProAccount(email, func(p *model.MailAccountProfile) {
			p.ProDeactivation = &model.ProDeactivation{Status: "queued", SubmittedAt: &past, AcceptedAt: &past, NextCheckAt: &past}
			switch state {
			case "stopped":
				p.ProAuto.StopRequested = true
			case "paused":
				p.ProMigration = &model.ProMigrationState{Paused: true}
			case "running":
				p.ProWorkflowRunning = true
			case "confirmed", "cleanup":
				p.ProDeactivation.ConfirmedAt, p.ProDeactivation.Status = &past, "completed"
				if state == "cleanup" {
					p.ProPostMergeGroups = &model.ProPostMergeGroups{Action: "deactivate", Status: "failed"}
				}
			}
		}); err != nil {
			t.Fatal(err)
		}
	}
	cfg.DeletionCheckIntervalSeconds, cfg.QuotaCheckIntervalSeconds = 37, 900
	before := time.Now()
	if _, _, err = s.SaveProAutomationSettings(cfg, gptpay.Settings{PlanCode: "pro5"}, ""); err != nil {
		t.Fatal(err)
	}
	after := time.Now()
	clocks := map[string]*time.Time{}
	for _, state := range states {
		p, _, err := s.MailAccountCredential(state + "@example.com")
		if err != nil {
			t.Fatal(err)
		}
		next := p.ProDeactivation.NextCheckAt
		clocks[state] = next
		switch state {
		case "confirmed":
			if next != nil {
				t.Fatal("confirmed account scheduled again")
			}
		case "running":
			if next == nil || !next.Equal(past) {
				t.Fatal("in-flight clock overwritten")
			}
		default:
			if next == nil || next.Before(before.Add(37*time.Second)) || next.After(after.Add(37*time.Second)) {
				t.Fatal("incorrect independent schedule", state, next)
			}
		}
		if state == "stopped" && !p.ProAuto.StopRequested || state == "paused" && !p.ProMigration.Paused {
			t.Fatal("save resumed paused work")
		}
	}
	// An unchanged save must not continually postpone pending confirmations.
	if _, _, err = s.SaveProAutomationSettings(cfg, gptpay.Settings{PlanCode: "pro5"}, ""); err != nil {
		t.Fatal(err)
	}
	p, _, _ := s.MailAccountCredential("pending@example.com")
	if !p.ProDeactivation.NextCheckAt.Equal(*clocks["pending"]) {
		t.Fatal("unchanged save postponed waiting task")
	}
	// Settings and rescheduling roll back together on failure.
	if _, err = s.db.Exec(`CREATE TRIGGER reject_clock BEFORE UPDATE ON mail_accounts BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	cfg.DeletionCheckIntervalSeconds = 55
	if _, _, err = s.SaveProAutomationSettings(cfg, gptpay.Settings{PlanCode: "pro20"}, ""); err == nil {
		t.Fatal("expected clock failure")
	}
	saved, _, _, _ := s.ProSettings()
	pay, _, _ := s.GPTPaySettings()
	if saved.DeletionCheckIntervalSeconds != 37 || pay.PlanCode != "pro5" {
		t.Fatal("partial settings save")
	}
	if _, err = s.db.Exec("DROP TRIGGER reject_clock"); err != nil {
		t.Fatal(err)
	}
	// Recovery uses the saved interval and still leaves completed work stopped.
	if _, err = s.UpdateProAccount("pending@example.com", func(p *model.MailAccountProfile) { p.ProDeactivation.NextCheckAt = nil }); err != nil {
		t.Fatal(err)
	}
	before = time.Now()
	if err = s.RecoverProWorkflows(); err != nil {
		t.Fatal(err)
	}
	p, _, _ = s.MailAccountCredential("pending@example.com")
	if p.ProDeactivation.NextCheckAt == nil || p.ProDeactivation.NextCheckAt.Before(before.Add(37*time.Second)) || p.ProDeactivation.NextCheckAt.After(time.Now().Add(37*time.Second)) {
		t.Fatal("restart ignored saved interval")
	}
	items, err := s.DueProDeactivations(time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range items {
		if p.Email == "confirmed@example.com" || p.Email == "stopped@example.com" || p.Email == "paused@example.com" {
			t.Fatal("ineligible task selected", p.Email)
		}
	}
}

func TestProDeletionStaleCompletedClocksDoNotStarveQueue(t *testing.T) {
	s := transferStore(t)
	now := time.Now()
	for i := 0; i < 12; i++ {
		email := fmt.Sprintf("stale-%02d@example.com", i)
		past := now.Add(time.Duration(i-60) * time.Minute)
		if _, err := s.SaveMailAccount(model.MailAccountProfile{Email: email}, model.MailAccountCredentials{Email: email}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.UpdateMailAccountManagementScope(email, "pro"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.UpdateProAccount(email, func(p *model.MailAccountProfile) {
			p.ProDeactivation = &model.ProDeactivation{SubmittedAt: &past, AcceptedAt: &past, NextCheckAt: &past}
			if i < 11 {
				p.ProDeactivation.ConfirmedAt = &past
			}
		}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := s.DueProDeactivations(now)
	if err != nil || len(items) != 1 || items[0].Email != "stale-11@example.com" {
		t.Fatal("completed clocks starved pending confirmation", items, err)
	}
}
