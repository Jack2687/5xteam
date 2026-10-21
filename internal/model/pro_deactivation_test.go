package model

import (
	"testing"
	"time"
)

func TestProDeletionDisplayAndCompletion(t *testing.T) {
	now := time.Now()
	p := MailAccountProfile{ManagementScope: "pro", ProAuto: ProAutoState{ID: "auto"}, ProDeactivation: &ProDeactivation{Status: "running", Steps: map[string]string{}}, ProPostMergeGroups: &ProPostMergeGroups{Action: "deactivate", Status: "pending"}}
	if p.ProStages().Steps["deactivate"] != "running" {
		t.Fatal("pending group plan falsely displayed as failure")
	}
	p.ProDeactivation.SubmittedAt, p.ProDeactivation.AcceptedAt = &now, &now
	p.ProDeactivation.Status = "queued"
	p.ReconcileProAutoDeletion(now)
	if p.ProAuto.Status != "awaiting_deletion" || p.ProStages().Steps["deactivate"] != "waiting" {
		t.Fatal("queued deletion shown as completed")
	}
	p.ProDeactivation.ConfirmedAt = &now
	p.ProDeactivation.Status = "completed"
	p.ReconcileProAutoDeletion(now)
	if p.ProAuto.Status != "awaiting_deletion" || p.ProStages().Steps["deactivate"] != "waiting" {
		t.Fatal("unfinished groups ignored")
	}
	p.ProPostMergeGroups.Status = "completed"
	p.ReconcileProAutoDeletion(now)
	if p.ProAuto.Status != "completed" {
		t.Fatal("completed cleanup not reflected")
	}
	p.ProAuto.StopRequested = true
	p.ReconcileProAutoDeletion(now)
	if p.ProAuto.Status != "stopped" {
		t.Fatal("stop request overwritten")
	}
}
