package cmd

import (
	"errors"
	"fastbrew/internal/daemon"
	"fmt"
	"time"
)

func tryRunMutationJob(commandName, operation string, packages []string, options daemon.JobSubmitOptions) (bool, error) {
	daemonClient, daemonErr := getDaemonClientForRead()
	if daemonClient == nil {
		if daemonErr != nil {
			warnDaemonFallback(commandName, daemonErr)
		}
		return false, nil
	}

	jobID, err := daemonClient.SubmitJob(operation, packages, options)
	if err != nil {
		warnDaemonFallback(commandName, err)
		return false, nil
	}

	if err := streamMutationJob(daemonClient, jobID); err != nil {
		return true, err
	}

	return true, nil
}

func streamMutationJob(client *daemon.Client, jobID string) error {
	fromSeq := 0
	for {
		stream, err := client.JobStream(jobID, fromSeq, true)
		if err != nil {
			return err
		}

		for _, event := range stream.Events {
			fmt.Println(formatMutationEvent(event))
			fromSeq = event.Seq + 1
		}

		switch stream.Job.Status {
		case daemon.JobStatusSucceeded:
			return nil
		case daemon.JobStatusFailed:
			if stream.Job.Error != "" {
				return errors.New(stream.Job.Error)
			}
			return fmt.Errorf("daemon job %s failed", jobID)
		default:
			if len(stream.Events) == 0 {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

func formatMutationEvent(event daemon.JobEvent) string {
	if event.Kind == daemon.JobEventKindPackage {
		return formatPackageEvent(event)
	}

	switch event.Level {
	case "warn", "warning":
		return fmt.Sprintf("⚠️  %s", event.Message)
	case "error":
		return fmt.Sprintf("❌ %s", event.Message)
	default:
		return event.Message
	}
}

func formatPackageEvent(event daemon.JobEvent) string {
	pkg := event.Package
	if pkg == "" {
		pkg = "unknown"
	}

	base := fmt.Sprintf("[%s]", pkg)
	if event.Phase != "" {
		base = fmt.Sprintf("%s %s", base, event.Phase)
	}
	if event.Status != "" {
		base = fmt.Sprintf("%s %s", base, event.Status)
	}

	if event.Status == daemon.JobEventStatusProgress && event.Current != nil && event.Total != nil && *event.Total > 0 {
		percent := float64(*event.Current) / float64(*event.Total) * 100
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
		return fmt.Sprintf("%s %.1f%%", base, percent)
	}

	if event.Message != "" {
		return fmt.Sprintf("%s - %s", base, event.Message)
	}
	return base
}
