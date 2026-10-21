package store

import (
	"testing"
	"time"

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
