package resume

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

type ValidationResult struct {
	Valid              bool
	RemoteChanged      bool
	CorruptionDetected bool
	MissingLocalFile   bool
	Errors             []error
}

func (vr *ValidationResult) AddError(err error) {
	vr.Errors = append(vr.Errors, err)
	vr.Valid = false
}

func ValidatePartialDownload(pd *PartialDownload, remoteLastModified, remoteETag string) *ValidationResult {
	result := &ValidationResult{
		Valid:              true,
		RemoteChanged:      false,
		CorruptionDetected: false,
		MissingLocalFile:   false,
		Errors:             make([]error, 0),
	}

	if pd == nil {
		result.AddError(fmt.Errorf("partial download is nil"))
		return result
	}

	if pd.LocalPath == "" {
		result.AddError(fmt.Errorf("local path is empty"))
		return result
	}

	fileInfo, err := os.Stat(pd.LocalPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.MissingLocalFile = true
			result.AddError(fmt.Errorf("local file does not exist: %s", pd.LocalPath))
		} else {
			result.AddError(fmt.Errorf("failed to stat local file: %w", err))
		}
		return result
	}

	actualSize := fileInfo.Size()
	if actualSize != pd.DownloadedBytes {
		result.CorruptionDetected = true
		result.AddError(fmt.Errorf("size mismatch: expected %d bytes, found %d bytes", pd.DownloadedBytes, actualSize))
	}

	if pd.LastModified != "" && remoteLastModified != "" {
		if pd.LastModified != remoteLastModified {
			result.RemoteChanged = true
			result.AddError(fmt.Errorf("last-modified mismatch: local=%s, remote=%s", pd.LastModified, remoteLastModified))
		}
	}

	if pd.ETag != "" && remoteETag != "" {
		if pd.ETag != remoteETag {
			result.RemoteChanged = true
			result.AddError(fmt.Errorf("etag mismatch: local=%s, remote=%s", pd.ETag, remoteETag))
		}
	}

	if pd.TotalSize > 0 && pd.DownloadedBytes > pd.TotalSize {
		result.CorruptionDetected = true
		result.AddError(fmt.Errorf("downloaded bytes exceed total size: %d > %d", pd.DownloadedBytes, pd.TotalSize))
	}

	if pd.State == StateComplete && pd.DownloadedBytes != pd.TotalSize {
		result.CorruptionDetected = true
		result.AddError(fmt.Errorf("completed download size mismatch: %d != %d", pd.DownloadedBytes, pd.TotalSize))
	}

	if pd.State == StateFailed {
		result.Valid = false
		result.AddError(fmt.Errorf("download is in failed state"))
	}

	return result
}

func CheckRemoteFileChanged(pd *PartialDownload, remoteLastModified, remoteETag string) bool {
	if pd == nil {
		return true
	}

	if pd.LastModified != "" && remoteLastModified != "" {
		if pd.LastModified != remoteLastModified {
			return true
		}
	}

	if pd.ETag != "" && remoteETag != "" {
		if pd.ETag != remoteETag {
			return true
		}
	}

	return false
}

func DetectCorruption(pd *PartialDownload) error {
	if pd == nil {
		return fmt.Errorf("partial download is nil")
	}

	if pd.LocalPath == "" {
		return fmt.Errorf("local path is empty")
	}

	fileInfo, err := os.Stat(pd.LocalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("local file does not exist: %s", pd.LocalPath)
		}
		return fmt.Errorf("failed to stat local file: %w", err)
	}

	actualSize := fileInfo.Size()
	if actualSize != pd.DownloadedBytes {
		return fmt.Errorf("size mismatch: expected %d bytes, found %d bytes", pd.DownloadedBytes, actualSize)
	}

	if pd.TotalSize > 0 && pd.DownloadedBytes > pd.TotalSize {
		return fmt.Errorf("downloaded bytes exceed total size: %d > %d", pd.DownloadedBytes, pd.TotalSize)
	}

	return nil
}

func ComputeFileChecksum(filepath string) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	buf := make([]byte, 8192)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			if _, err := hash.Write(buf[:n]); err != nil {
				return "", fmt.Errorf("failed to compute hash: %w", err)
			}
		}
		if err != nil {
			break
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func ValidateChecksum(filepath, expectedChecksum string) error {
	if expectedChecksum == "" {
		return nil
	}

	actualChecksum, err := ComputeFileChecksum(filepath)
	if err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return nil
}

func CanResumeDownload(pd *PartialDownload, remoteLastModified, remoteETag string) bool {
	if pd == nil {
		return false
	}

	if !CanResume(pd.State) {
		return false
	}

	if pd.LocalPath == "" {
		return false
	}

	if _, err := os.Stat(pd.LocalPath); err != nil {
		return false
	}

	if CheckRemoteFileChanged(pd, remoteLastModified, remoteETag) {
		return false
	}

	if err := DetectCorruption(pd); err != nil {
		return false
	}

	return true
}

func GetResumeOffset(pd *PartialDownload) int64 {
	if pd == nil {
		return 0
	}

	if !CanResume(pd.State) {
		return 0
	}

	if pd.DownloadedBytes >= pd.TotalSize {
		return 0
	}

	return pd.DownloadedBytes
}
