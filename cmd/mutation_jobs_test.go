package cmd

import (
	"fastbrew/internal/daemon"
	"strings"
	"testing"
)

func TestFormatMutationEventStructuredPackageProgress(t *testing.T) {
	current := int64(25)
	total := int64(50)
	out := formatMutationEvent(daemon.JobEvent{
		Kind:    daemon.JobEventKindPackage,
		Package: "jq",
		Phase:   daemon.JobEventPhaseDownload,
		Status:  daemon.JobEventStatusProgress,
		Current: &current,
		Total:   &total,
	})

	if !strings.Contains(out, "[jq]") {
		t.Fatalf("expected package label in output, got %q", out)
	}
	if !strings.Contains(out, "download") {
		t.Fatalf("expected phase in output, got %q", out)
	}
	if !strings.Contains(out, "50.0%") {
		t.Fatalf("expected percentage in output, got %q", out)
	}
}

func TestFormatMutationEventLegacyFallback(t *testing.T) {
	out := formatMutationEvent(daemon.JobEvent{
		Level:   "warn",
		Message: "legacy warning",
	})

	if !strings.Contains(out, "legacy warning") {
		t.Fatalf("expected legacy message output, got %q", out)
	}
	if !strings.Contains(out, "⚠️") {
		t.Fatalf("expected warning prefix, got %q", out)
	}
}
