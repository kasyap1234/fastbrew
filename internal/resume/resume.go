package resume

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const ResumeMetadataSuffix = ".fastbrew-resume"

type PartialDownload struct {
	URL             string            `json:"url"`
	LocalPath       string            `json:"local_path"`
	TotalSize       int64             `json:"total_size"`
	DownloadedBytes int64             `json:"downloaded_bytes"`
	Checksum        string            `json:"checksum"`
	LastModified    string            `json:"last_modified"`
	ETag            string            `json:"etag"`
	State           DownloadState     `json:"state"`
	StateHistory    []StateTransition `json:"state_history"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type StateTransition struct {
	FromState string    `json:"from_state"`
	ToState   string    `json:"to_state"`
	Timestamp time.Time `json:"timestamp"`
}

func (pd *PartialDownload) MetadataPath() string {
	return pd.LocalPath + ResumeMetadataSuffix
}

func (pd *PartialDownload) ComputePartialChecksum() (string, error) {
	file, err := os.Open(pd.LocalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := hash.Write([]byte(pd.URL + pd.LastModified + pd.ETag)); err != nil {
		return "", fmt.Errorf("failed to write to hash: %w", err)
	}

	stat, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	if stat.Size() > 0 {
		buf := make([]byte, 8192)
		for {
			n, err := file.Read(buf)
			if n > 0 {
				if _, err := hash.Write(buf[:n]); err != nil {
					return "", fmt.Errorf("failed to write to hash: %w", err)
				}
			}
			if err != nil {
				break
			}
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (pd *PartialDownload) UpdateState(newState DownloadState) error {
	if err := ValidateStateTransition(pd.State, newState); err != nil {
		return err
	}

	transition := StateTransition{
		FromState: pd.State.String(),
		ToState:   newState.String(),
		Timestamp: time.Now(),
	}

	pd.StateHistory = append(pd.StateHistory, transition)
	pd.State = newState
	pd.UpdatedAt = time.Now()

	return nil
}

func (pd *PartialDownload) CalculateProgress() float64 {
	if pd.TotalSize == 0 {
		return 0.0
	}
	return float64(pd.DownloadedBytes) / float64(pd.TotalSize) * 100.0
}

func (pd *PartialDownload) IsComplete() bool {
	return pd.State == StateComplete
}

func (pd *PartialDownload) IsValid() bool {
	return pd.State != StateFailed && pd.DownloadedBytes <= pd.TotalSize
}

type ResumeManager struct {
	baseDir string
}

func NewResumeManager(baseDir string) *ResumeManager {
	return &ResumeManager{
		baseDir: baseDir,
	}
}

func (rm *ResumeManager) Create(url, path string) (*PartialDownload, error) {
	if url == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}
	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	metadataPath := absPath + ResumeMetadataSuffix
	if _, err := os.Stat(metadataPath); err == nil {
		return nil, fmt.Errorf("resume metadata already exists: %s", metadataPath)
	}

	now := time.Now()
	pd := &PartialDownload{
		URL:             url,
		LocalPath:       absPath,
		TotalSize:       0,
		DownloadedBytes: 0,
		Checksum:        "",
		LastModified:    "",
		ETag:            "",
		State:           StatePending,
		StateHistory: []StateTransition{
			{
				FromState: "",
				ToState:   StatePending.String(),
				Timestamp: now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := rm.Save(pd); err != nil {
		return nil, fmt.Errorf("failed to save initial resume metadata: %w", err)
	}

	return pd, nil
}

func (rm *ResumeManager) Load(path string) (*PartialDownload, error) {
	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	metadataPath := absPath + ResumeMetadataSuffix
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("resume metadata not found: %s", metadataPath)
		}
		return nil, fmt.Errorf("failed to read resume metadata: %w", err)
	}

	var pd PartialDownload
	if err := json.Unmarshal(data, &pd); err != nil {
		return nil, fmt.Errorf("failed to parse resume metadata: %w", err)
	}

	pd.LocalPath = absPath

	return &pd, nil
}

func (rm *ResumeManager) Save(pd *PartialDownload) error {
	if pd == nil {
		return fmt.Errorf("partial download cannot be nil")
	}

	pd.UpdatedAt = time.Now()

	metadataPath := pd.MetadataPath()
	dir := filepath.Dir(metadataPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(pd, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal resume metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write resume metadata: %w", err)
	}

	return nil
}

func (rm *ResumeManager) Delete(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	metadataPath := absPath + ResumeMetadataSuffix
	if err := os.Remove(metadataPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("resume metadata not found: %s", metadataPath)
		}
		return fmt.Errorf("failed to delete resume metadata: %w", err)
	}

	return nil
}

func (rm *ResumeManager) Exists(path string) bool {
	if path == "" {
		return false
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	metadataPath := absPath + ResumeMetadataSuffix
	_, err = os.Stat(metadataPath)
	return err == nil
}

func (rm *ResumeManager) List() ([]*PartialDownload, error) {
	var downloads []*PartialDownload

	entries, err := os.ReadDir(rm.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return downloads, nil
		}
		return nil, fmt.Errorf("failed to read base directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) == ResumeMetadataSuffix {
			path := filepath.Join(rm.baseDir, name[:len(name)-len(ResumeMetadataSuffix)])
			pd, err := rm.Load(path)
			if err != nil {
				continue
			}
			downloads = append(downloads, pd)
		}
	}

	return downloads, nil
}
