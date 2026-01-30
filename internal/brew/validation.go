package brew

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

type CacheValidator struct {
	cacheDir string
}

func NewCacheValidator(cacheDir string) *CacheValidator {
	return &CacheValidator{cacheDir: cacheDir}
}

type CacheStatus struct {
	Path      string
	Valid     bool
	Size      int64
	Checksum  string
	Error     error
}

func (v *CacheValidator) ValidateAll() ([]CacheStatus, error) {
	var statuses []CacheStatus

	formulaStatus := v.ValidateFormulaCache()
	statuses = append(statuses, formulaStatus)

	caskStatus := v.ValidateCaskCache()
	statuses = append(statuses, caskStatus)

	searchStatus := v.ValidateSearchCache()
	statuses = append(statuses, searchStatus)

	return statuses, nil
}

func (v *CacheValidator) ValidateFormulaCache() CacheStatus {
	path := fmt.Sprintf("%s/formula.json.zst", v.cacheDir)
	return v.validateJSONCache(path)
}

func (v *CacheValidator) ValidateCaskCache() CacheStatus {
	path := fmt.Sprintf("%s/cask.json.zst", v.cacheDir)
	return v.validateJSONCache(path)
}

func (v *CacheValidator) ValidateSearchCache() CacheStatus {
	path := fmt.Sprintf("%s/search.gob.zst", v.cacheDir)
	return v.validateGOBCache(path)
}

func (v *CacheValidator) validateJSONCache(path string) CacheStatus {
	status := CacheStatus{Path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		status.Error = fmt.Errorf("failed to read: %w", err)
		return status
	}

	status.Size = int64(len(data))
	hash := sha256.Sum256(data)
	status.Checksum = hex.EncodeToString(hash[:])

	decompressed, err := decompressWithPool(data)
	if err != nil {
		status.Error = fmt.Errorf("decompression failed: %w", err)
		return status
	}

	var result interface{}
	if err := json.Unmarshal(decompressed, &result); err != nil {
		status.Error = fmt.Errorf("json validation failed: %w", err)
		return status
	}

	status.Valid = true
	return status
}

func (v *CacheValidator) validateGOBCache(path string) CacheStatus {
	status := CacheStatus{Path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		status.Error = fmt.Errorf("failed to read: %w", err)
		return status
	}

	status.Size = int64(len(data))
	hash := sha256.Sum256(data)
	status.Checksum = hex.EncodeToString(hash[:])

	_, err = decompressWithPool(data)
	if err != nil {
		status.Error = fmt.Errorf("decompression failed: %w", err)
		return status
	}

	status.Valid = true
	return status
}
