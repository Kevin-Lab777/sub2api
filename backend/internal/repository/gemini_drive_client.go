package repository

import "github.com/Kevin-Lab777/sub2api/internal/pkg/geminicli"

// NewGeminiDriveClient creates a concrete DriveClient for Google Drive API operations.
// Returned as geminicli.DriveClient interface for DI (Strategy A).
func NewGeminiDriveClient() geminicli.DriveClient {
	return geminicli.NewDriveClient()
}
