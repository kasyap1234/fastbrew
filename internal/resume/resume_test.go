package resume

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPartialDownload_MetadataPath(t *testing.T) {
	pd := &PartialDownload{
		LocalPath: "/tmp/test.file",
	}

	expected := "/tmp/test.file.fastbrew-resume"
	if pd.MetadataPath() != expected {
		t.Errorf("MetadataPath() = %s, want %s", pd.MetadataPath(), expected)
	}
}

func TestPartialDownload_UpdateState(t *testing.T) {
	pd := &PartialDownload{
		State: StatePending,
		StateHistory: []StateTransition{
			{FromState: "", ToState: "pending", Timestamp: time.Now()},
		},
	}

	if err := pd.UpdateState(StateInProgress); err != nil {
		t.Errorf("UpdateState(StateInProgress) error = %v", err)
	}

	if pd.State != StateInProgress {
		t.Errorf("State = %v, want %v", pd.State, StateInProgress)
	}

	if len(pd.StateHistory) != 2 {
		t.Errorf("len(StateHistory) = %d, want 2", len(pd.StateHistory))
	}

	if err := pd.UpdateState(StatePending); err != nil {
		t.Errorf("UpdateState(StatePending) from StateInProgress error = %v", err)
	}

	if pd.State != StatePending {
		t.Errorf("State = %v, want %v", pd.State, StatePending)
	}

	if len(pd.StateHistory) != 3 {
		t.Errorf("len(StateHistory) = %d, want 3", len(pd.StateHistory))
	}

	if err := pd.UpdateState(StateInProgress); err != nil {
		t.Errorf("UpdateState(StateInProgress) from StatePending error = %v", err)
	}

	if err := pd.UpdateState(StateComplete); err != nil {
		t.Errorf("UpdateState(StateComplete) from StateInProgress error = %v", err)
	}
}

func TestPartialDownload_CalculateProgress(t *testing.T) {
	tests := []struct {
		name            string
		totalSize       int64
		downloadedBytes int64
		want            float64
	}{
		{"empty", 0, 0, 0.0},
		{"half", 100, 50, 50.0},
		{"complete", 100, 100, 100.0},
		{"none", 100, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &PartialDownload{
				TotalSize:       tt.totalSize,
				DownloadedBytes: tt.downloadedBytes,
			}

			got := pd.CalculateProgress()
			if got != tt.want {
				t.Errorf("CalculateProgress() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestResumeManager_Create(t *testing.T) {
	tempDir := t.TempDir()
	rm := NewResumeManager(tempDir)

	url := "https://example.com/file.tar.gz"
	path := filepath.Join(tempDir, "file.tar.gz")

	pd, err := rm.Create(url, path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if pd.URL != url {
		t.Errorf("URL = %s, want %s", pd.URL, url)
	}

	if pd.State != StatePending {
		t.Errorf("State = %v, want %v", pd.State, StatePending)
	}

	if !rm.Exists(path) {
		t.Error("Exists() should return true after Create()")
	}

	_, err = rm.Create(url, path)
	if err == nil {
		t.Error("Create() should fail when metadata already exists")
	}
}

func TestResumeManager_Create_InvalidInput(t *testing.T) {
	tempDir := t.TempDir()
	rm := NewResumeManager(tempDir)

	if _, err := rm.Create("", "/tmp/file"); err == nil {
		t.Error("Create() should fail with empty URL")
	}

	if _, err := rm.Create("http://example.com", ""); err == nil {
		t.Error("Create() should fail with empty path")
	}
}

func TestResumeManager_Load(t *testing.T) {
	tempDir := t.TempDir()
	rm := NewResumeManager(tempDir)

	url := "https://example.com/file.tar.gz"
	path := filepath.Join(tempDir, "file.tar.gz")

	pd, err := rm.Create(url, path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	pd.TotalSize = 1000
	pd.DownloadedBytes = 500
	pd.State = StateInProgress
	if err := rm.Save(pd); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := rm.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.URL != url {
		t.Errorf("URL = %s, want %s", loaded.URL, url)
	}

	if loaded.TotalSize != 1000 {
		t.Errorf("TotalSize = %d, want 1000", loaded.TotalSize)
	}

	if loaded.DownloadedBytes != 500 {
		t.Errorf("DownloadedBytes = %d, want 500", loaded.DownloadedBytes)
	}

	if loaded.State != StateInProgress {
		t.Errorf("State = %v, want %v", loaded.State, StateInProgress)
	}
}

func TestResumeManager_Load_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	rm := NewResumeManager(tempDir)

	path := filepath.Join(tempDir, "nonexistent.file")

	_, err := rm.Load(path)
	if err == nil {
		t.Error("Load() should fail when metadata does not exist")
	}
}

func TestResumeManager_Save(t *testing.T) {
	tempDir := t.TempDir()
	rm := NewResumeManager(tempDir)

	url := "https://example.com/file.tar.gz"
	path := filepath.Join(tempDir, "file.tar.gz")

	pd, err := rm.Create(url, path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	pd.DownloadedBytes = 100
	pd.State = StateInProgress

	if err := rm.Save(pd); err != nil {
		t.Errorf("Save() error = %v", err)
	}

	metadataPath := path + ResumeMetadataSuffix
	if _, err := os.Stat(metadataPath); err != nil {
		t.Errorf("metadata file should exist after Save()")
	}
}

func TestResumeManager_Save_Nil(t *testing.T) {
	tempDir := t.TempDir()
	rm := NewResumeManager(tempDir)

	if err := rm.Save(nil); err == nil {
		t.Error("Save() should fail with nil partial download")
	}
}

func TestResumeManager_Delete(t *testing.T) {
	tempDir := t.TempDir()
	rm := NewResumeManager(tempDir)

	url := "https://example.com/file.tar.gz"
	path := filepath.Join(tempDir, "file.tar.gz")

	if _, err := rm.Create(url, path); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := rm.Delete(path); err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	if rm.Exists(path) {
		t.Error("Exists() should return false after Delete()")
	}
}

func TestResumeManager_Delete_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	rm := NewResumeManager(tempDir)

	path := filepath.Join(tempDir, "nonexistent.file")

	if err := rm.Delete(path); err == nil {
		t.Error("Delete() should fail when metadata does not exist")
	}
}

func TestResumeManager_Exists(t *testing.T) {
	tempDir := t.TempDir()
	rm := NewResumeManager(tempDir)

	if rm.Exists("") {
		t.Error("Exists(\"\") should return false")
	}

	path := filepath.Join(tempDir, "file.tar.gz")
	if rm.Exists(path) {
		t.Error("Exists() should return false for non-existent file")
	}

	if _, err := rm.Create("http://example.com", path); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !rm.Exists(path) {
		t.Error("Exists() should return true for existing file")
	}
}

func TestState_Constants(t *testing.T) {
	if StatePending.String() != "pending" {
		t.Errorf("StatePending.String() = %s, want pending", StatePending.String())
	}

	if StateInProgress.String() != "in_progress" {
		t.Errorf("StateInProgress.String() = %s, want in_progress", StateInProgress.String())
	}

	if StateComplete.String() != "complete" {
		t.Errorf("StateComplete.String() = %s, want complete", StateComplete.String())
	}

	if StateFailed.String() != "failed" {
		t.Errorf("StateFailed.String() = %s, want failed", StateFailed.String())
	}
}

func TestState_ParseState(t *testing.T) {
	tests := []struct {
		input    string
		expected DownloadState
	}{
		{"pending", StatePending},
		{"in_progress", StateInProgress},
		{"complete", StateComplete},
		{"failed", StateFailed},
		{"unknown", StateFailed},
	}

	for _, tt := range tests {
		got := ParseState(tt.input)
		if got != tt.expected {
			t.Errorf("ParseState(%s) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestValidateStateTransition(t *testing.T) {
	tests := []struct {
		from    DownloadState
		to      DownloadState
		wantErr bool
	}{
		{StatePending, StateInProgress, false},
		{StatePending, StateFailed, false},
		{StatePending, StateComplete, true},
		{StateInProgress, StateComplete, false},
		{StateInProgress, StateFailed, false},
		{StateInProgress, StatePending, false},
		{StateComplete, StatePending, true},
		{StateFailed, StatePending, false},
		{StateFailed, StateInProgress, false},
		{StatePending, StatePending, false},
	}

	for _, tt := range tests {
		err := ValidateStateTransition(tt.from, tt.to)
		if tt.wantErr && err == nil {
			t.Errorf("ValidateStateTransition(%v, %v) should error", tt.from, tt.to)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("ValidateStateTransition(%v, %v) error = %v", tt.from, tt.to, err)
		}
	}
}

func TestIsTerminalState(t *testing.T) {
	if !IsTerminalState(StateComplete) {
		t.Error("IsTerminalState(StateComplete) should be true")
	}

	if !IsTerminalState(StateFailed) {
		t.Error("IsTerminalState(StateFailed) should be true")
	}

	if IsTerminalState(StatePending) {
		t.Error("IsTerminalState(StatePending) should be false")
	}

	if IsTerminalState(StateInProgress) {
		t.Error("IsTerminalState(StateInProgress) should be false")
	}
}

func TestCanResume(t *testing.T) {
	if !CanResume(StateFailed) {
		t.Error("CanResume(StateFailed) should be true")
	}

	if !CanResume(StatePending) {
		t.Error("CanResume(StatePending) should be true")
	}

	if CanResume(StateComplete) {
		t.Error("CanResume(StateComplete) should be false")
	}

	if CanResume(StateInProgress) {
		t.Error("CanResume(StateInProgress) should be false")
	}
}

func TestStateTracker(t *testing.T) {
	st := NewStateTracker(StatePending)

	if st.CurrentState != StatePending {
		t.Errorf("CurrentState = %v, want %v", st.CurrentState, StatePending)
	}

	if len(st.History) != 1 {
		t.Errorf("len(History) = %d, want 1", len(st.History))
	}

	if err := st.Transition(StateInProgress); err != nil {
		t.Errorf("Transition(StateInProgress) error = %v", err)
	}

	if st.CurrentState != StateInProgress {
		t.Errorf("CurrentState = %v, want %v", st.CurrentState, StateInProgress)
	}

	if len(st.History) != 2 {
		t.Errorf("len(History) = %d, want 2", len(st.History))
	}

	if err := st.Transition(StateComplete); err != nil {
		t.Errorf("Transition(StateComplete) error = %v", err)
	}

	if st.GetStateCount(StatePending) != 1 {
		t.Errorf("GetStateCount(StatePending) = %d, want 1", st.GetStateCount(StatePending))
	}

	if st.GetStateCount(StateInProgress) != 1 {
		t.Errorf("GetStateCount(StateInProgress) = %d, want 1", st.GetStateCount(StateInProgress))
	}

	if st.GetStateCount(StateComplete) != 1 {
		t.Errorf("GetStateCount(StateComplete) = %d, want 1", st.GetStateCount(StateComplete))
	}
}

func TestValidatePartialDownload(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.file")

	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	pd := &PartialDownload{
		LocalPath:       testFile,
		TotalSize:       100,
		DownloadedBytes: 12,
		LastModified:    "Mon, 01 Jan 2024 00:00:00 GMT",
		ETag:            `"abc123"`,
		State:           StateInProgress,
	}

	result := ValidatePartialDownload(pd, "Mon, 01 Jan 2024 00:00:00 GMT", `"abc123"`)
	if !result.Valid {
		t.Errorf("ValidatePartialDownload() should be valid, got errors: %v", result.Errors)
	}

	result = ValidatePartialDownload(pd, "Tue, 02 Jan 2024 00:00:00 GMT", `"abc123"`)
	if !result.RemoteChanged {
		t.Error("ValidatePartialDownload() should detect remote change")
	}

	result = ValidatePartialDownload(pd, "Mon, 01 Jan 2024 00:00:00 GMT", `"xyz789"`)
	if !result.RemoteChanged {
		t.Error("ValidatePartialDownload() should detect ETag change")
	}
}

func TestValidatePartialDownload_MissingFile(t *testing.T) {
	pd := &PartialDownload{
		LocalPath:       "/nonexistent/path/file",
		DownloadedBytes: 0,
	}

	result := ValidatePartialDownload(pd, "", "")
	if !result.MissingLocalFile {
		t.Error("ValidatePartialDownload() should detect missing local file")
	}
}

func TestValidatePartialDownload_Nil(t *testing.T) {
	result := ValidatePartialDownload(nil, "", "")
	if result.Valid {
		t.Error("ValidatePartialDownload(nil) should not be valid")
	}
}

func TestCheckRemoteFileChanged(t *testing.T) {
	pd := &PartialDownload{
		LastModified: "Mon, 01 Jan 2024 00:00:00 GMT",
		ETag:         `"abc123"`,
	}

	if CheckRemoteFileChanged(pd, "Mon, 01 Jan 2024 00:00:00 GMT", `"abc123"`) {
		t.Error("CheckRemoteFileChanged() should return false for matching values")
	}

	if !CheckRemoteFileChanged(pd, "Tue, 02 Jan 2024 00:00:00 GMT", `"abc123"`) {
		t.Error("CheckRemoteFileChanged() should return true for different Last-Modified")
	}

	if !CheckRemoteFileChanged(pd, "Mon, 01 Jan 2024 00:00:00 GMT", `"xyz789"`) {
		t.Error("CheckRemoteFileChanged() should return true for different ETag")
	}

	if !CheckRemoteFileChanged(nil, "", "") {
		t.Error("CheckRemoteFileChanged(nil) should return true")
	}
}

func TestDetectCorruption(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.file")

	content := []byte("test content for checksum")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	pd := &PartialDownload{
		LocalPath:       testFile,
		DownloadedBytes: int64(len(content)),
		TotalSize:       100,
	}

	if err := DetectCorruption(pd); err != nil {
		t.Errorf("DetectCorruption() error = %v", err)
	}

	pd.DownloadedBytes = 999
	if err := DetectCorruption(pd); err == nil {
		t.Error("DetectCorruption() should detect size mismatch")
	}

	pd.DownloadedBytes = 150
	pd.TotalSize = 100
	if err := DetectCorruption(pd); err == nil {
		t.Error("DetectCorruption() should detect bytes exceeding total size")
	}

	pd.LocalPath = "/nonexistent"
	if err := DetectCorruption(pd); err == nil {
		t.Error("DetectCorruption() should error for missing file")
	}

	pd.LocalPath = ""
	if err := DetectCorruption(pd); err == nil {
		t.Error("DetectCorruption() should error for empty path")
	}

	if err := DetectCorruption(nil); err == nil {
		t.Error("DetectCorruption(nil) should error")
	}
}

func TestComputeFileChecksum(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.file")

	content := []byte("test content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	checksum1, err := ComputeFileChecksum(testFile)
	if err != nil {
		t.Errorf("ComputeFileChecksum() error = %v", err)
	}

	if checksum1 == "" {
		t.Error("ComputeFileChecksum() should return non-empty checksum")
	}

	checksum2, err := ComputeFileChecksum(testFile)
	if err != nil {
		t.Errorf("ComputeFileChecksum() error = %v", err)
	}

	if checksum1 != checksum2 {
		t.Error("ComputeFileChecksum() should return consistent checksum")
	}

	_, err = ComputeFileChecksum("/nonexistent/file")
	if err == nil {
		t.Error("ComputeFileChecksum() should error for non-existent file")
	}
}

func TestValidateChecksum(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.file")

	content := []byte("test content for validation")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if err := ValidateChecksum(testFile, ""); err != nil {
		t.Errorf("ValidateChecksum() with empty expected should not error: %v", err)
	}

	expectedChecksum, _ := ComputeFileChecksum(testFile)

	if err := ValidateChecksum(testFile, expectedChecksum); err != nil {
		t.Errorf("ValidateChecksum() with correct checksum error = %v", err)
	}

	if err := ValidateChecksum(testFile, "invalid_checksum"); err == nil {
		t.Error("ValidateChecksum() with wrong checksum should error")
	}
}

func TestCanResumeDownload(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.file")

	content := make([]byte, 50)
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	pd := &PartialDownload{
		LocalPath:       testFile,
		TotalSize:       100,
		DownloadedBytes: 50,
		LastModified:    "Mon, 01 Jan 2024 00:00:00 GMT",
		ETag:            `"abc123"`,
		State:           StateFailed,
	}

	if !CanResumeDownload(pd, "Mon, 01 Jan 2024 00:00:00 GMT", `"abc123"`) {
		t.Error("CanResumeDownload() should return true for valid failed download")
	}

	pd.State = StatePending
	if !CanResumeDownload(pd, "Mon, 01 Jan 2024 00:00:00 GMT", `"abc123"`) {
		t.Error("CanResumeDownload() should return true for valid pending download")
	}

	pd.State = StateComplete
	if CanResumeDownload(pd, "Mon, 01 Jan 2024 00:00:00 GMT", `"abc123"`) {
		t.Error("CanResumeDownload() should return false for complete download")
	}

	if CanResumeDownload(nil, "", "") {
		t.Error("CanResumeDownload(nil) should return false")
	}

	pd.State = StateFailed
	if CanResumeDownload(pd, "Tue, 02 Jan 2024 00:00:00 GMT", `"abc123"`) {
		t.Error("CanResumeDownload() should return false when remote changed")
	}
}

func TestGetResumeOffset(t *testing.T) {
	pd := &PartialDownload{
		TotalSize:       100,
		DownloadedBytes: 50,
		State:           StateFailed,
	}

	if GetResumeOffset(pd) != 50 {
		t.Errorf("GetResumeOffset() = %d, want 50", GetResumeOffset(pd))
	}

	pd.State = StateComplete
	if GetResumeOffset(pd) != 0 {
		t.Errorf("GetResumeOffset() for complete = %d, want 0", GetResumeOffset(pd))
	}

	pd.DownloadedBytes = 150
	pd.State = StateFailed
	if GetResumeOffset(pd) != 0 {
		t.Errorf("GetResumeOffset() for overflow = %d, want 0", GetResumeOffset(pd))
	}

	if GetResumeOffset(nil) != 0 {
		t.Errorf("GetResumeOffset(nil) = %d, want 0", GetResumeOffset(nil))
	}
}

func TestPartialDownload_IsComplete(t *testing.T) {
	pd := &PartialDownload{State: StateComplete}
	if !pd.IsComplete() {
		t.Error("IsComplete() should return true for StateComplete")
	}

	pd.State = StateInProgress
	if pd.IsComplete() {
		t.Error("IsComplete() should return false for StateInProgress")
	}
}

func TestPartialDownload_IsValid(t *testing.T) {
	pd := &PartialDownload{
		State:           StateInProgress,
		DownloadedBytes: 50,
		TotalSize:       100,
	}
	if !pd.IsValid() {
		t.Error("IsValid() should return true for valid in-progress download")
	}

	pd.State = StateFailed
	if pd.IsValid() {
		t.Error("IsValid() should return false for failed download")
	}

	pd.State = StateInProgress
	pd.DownloadedBytes = 150
	if pd.IsValid() {
		t.Error("IsValid() should return false when bytes exceed total")
	}
}

func TestPartialDownload_ComputePartialChecksum(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.file")

	content := []byte("test content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	pd := &PartialDownload{
		LocalPath:    testFile,
		URL:          "https://example.com/file",
		LastModified: "Mon, 01 Jan 2024 00:00:00 GMT",
		ETag:         `"abc123"`,
	}

	checksum1, err := pd.ComputePartialChecksum()
	if err != nil {
		t.Errorf("ComputePartialChecksum() error = %v", err)
	}

	if checksum1 == "" {
		t.Error("ComputePartialChecksum() should return non-empty checksum")
	}

	checksum2, err := pd.ComputePartialChecksum()
	if err != nil {
		t.Errorf("ComputePartialChecksum() error = %v", err)
	}

	if checksum1 != checksum2 {
		t.Error("ComputePartialChecksum() should return consistent checksum")
	}

	pd.LocalPath = "/nonexistent/file"
	checksum, err := pd.ComputePartialChecksum()
	if err != nil {
		t.Errorf("ComputePartialChecksum() for new file should not error: %v", err)
	}
	if checksum != "" {
		t.Error("ComputePartialChecksum() for non-existent file should return empty checksum")
	}
}

func TestStateTracker_GetLastTransition(t *testing.T) {
	st := NewStateTracker(StatePending)

	last := st.GetLastTransition()
	if last == nil {
		t.Fatal("GetLastTransition() should not return nil")
	}

	if last.ToState != "pending" {
		t.Errorf("LastTransition.ToState = %s, want pending", last.ToState)
	}

	st.Transition(StateInProgress)
	last = st.GetLastTransition()
	if last.ToState != "in_progress" {
		t.Errorf("LastTransition.ToState = %s, want in_progress", last.ToState)
	}
}

func TestStateTracker_GetTimeInState(t *testing.T) {
	st := NewStateTracker(StatePending)

	time.Sleep(10 * time.Millisecond)

	duration := st.GetTimeInState()
	if duration < 10*time.Millisecond {
		t.Error("GetTimeInState() should return meaningful duration")
	}

	total := st.GetTotalTime()
	if total < duration {
		t.Error("GetTotalTime() should be >= GetTimeInState()")
	}
}

func TestResumeManager_List(t *testing.T) {
	tempDir := t.TempDir()
	rm := NewResumeManager(tempDir)

	downloads, err := rm.List()
	if err != nil {
		t.Errorf("List() error = %v", err)
	}

	if len(downloads) != 0 {
		t.Errorf("List() should return empty slice for empty directory, got %d", len(downloads))
	}

	if _, err := rm.Create("http://example.com/1", filepath.Join(tempDir, "file1")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := rm.Create("http://example.com/2", filepath.Join(tempDir, "file2")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	downloads, err = rm.List()
	if err != nil {
		t.Errorf("List() error = %v", err)
	}

	if len(downloads) != 2 {
		t.Errorf("List() should return 2 downloads, got %d", len(downloads))
	}
}
