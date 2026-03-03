package tui

import (
	"fastbrew/internal/brew"
	"fastbrew/internal/daemon"
	"fastbrew/internal/progress"
	"testing"
)

func TestApplyJobEventUpdatesPackageProgress(t *testing.T) {
	m := InitialModel()
	m.jobPackages = make(map[string]*packageProgress)

	current := int64(50)
	total := int64(100)
	m.applyJobEvent(daemon.JobEvent{
		Kind:    daemon.JobEventKindPackage,
		Package: "jq",
		Phase:   daemon.JobEventPhaseDownload,
		Status:  daemon.JobEventStatusProgress,
		Current: &current,
		Total:   &total,
	})

	pkg, ok := m.jobPackages["jq"]
	if !ok {
		t.Fatal("expected package entry to be created")
	}
	if pkg.Phase != daemon.JobEventPhaseDownload {
		t.Fatalf("expected phase %q, got %q", daemon.JobEventPhaseDownload, pkg.Phase)
	}
	if pkg.Status != daemon.JobEventStatusProgress {
		t.Fatalf("expected status %q, got %q", daemon.JobEventStatusProgress, pkg.Status)
	}
	if pkg.Percent != 50 {
		t.Fatalf("expected percent 50, got %.2f", pkg.Percent)
	}

	m.applyJobEvent(daemon.JobEvent{
		Kind:    daemon.JobEventKindPackage,
		Package: "jq",
		Phase:   daemon.JobEventPhaseComplete,
		Status:  daemon.JobEventStatusSucceeded,
	})

	if pkg.Percent != 100 {
		t.Fatalf("expected percent 100 after success, got %.2f", pkg.Percent)
	}
}

func TestJobFinishedReenablesActionsAndUpdatesInstalled(t *testing.T) {
	m := InitialModel()
	m.jobActive = true
	m.jobVisible = true
	m.index = &brew.Index{}

	updatedModel, _ := m.Update(jobFinishedMsg{
		Installed: map[string]bool{"jq": true},
	})
	updated := updatedModel.(*model)

	if updated.jobActive {
		t.Fatal("expected jobActive=false after finish")
	}
	if !updated.installed["jq"] {
		t.Fatal("expected installed marker for jq to be refreshed")
	}
}

func TestProgressAndMutationEventMappings(t *testing.T) {
	progressEvent := progressToJobEvent(progress.ProgressEvent{
		Type:    progress.EventDownloadProgress,
		ID:      "wget",
		Message: "Downloading...",
		Current: 25,
		Total:   50,
	})
	if progressEvent.Kind != daemon.JobEventKindPackage {
		t.Fatalf("expected package event kind, got %q", progressEvent.Kind)
	}
	if progressEvent.Status != daemon.JobEventStatusProgress {
		t.Fatalf("expected progress status, got %q", progressEvent.Status)
	}
	if progressEvent.Package != "wget" {
		t.Fatalf("expected package wget, got %q", progressEvent.Package)
	}

	mutationEvent := mutationToJobEvent(brew.MutationEvent{
		Operation: brew.MutationOperationInstall,
		Package:   "wget",
		Phase:     brew.MutationPhaseExtract,
		Status:    brew.MutationStatusRunning,
		Message:   "extracting",
	})
	if mutationEvent.Phase != brew.MutationPhaseExtract {
		t.Fatalf("expected phase %q, got %q", brew.MutationPhaseExtract, mutationEvent.Phase)
	}
	if mutationEvent.Status != brew.MutationStatusRunning {
		t.Fatalf("expected status %q, got %q", brew.MutationStatusRunning, mutationEvent.Status)
	}
	if mutationEvent.Level != "info" {
		t.Fatalf("expected level info, got %q", mutationEvent.Level)
	}
}
