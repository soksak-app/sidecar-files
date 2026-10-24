package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed sidecar.json
var sidecarManifest embed.FS

// ProcessDisplayName returns the process identity declared by the manifest embedded in the executable.
func ProcessDisplayName() (string, error) {
	data, err := sidecarManifest.ReadFile("sidecar.json")
	if err != nil {
		return "", fmt.Errorf("sidecar.json: read embedded manifest: %w", err)
	}
	var manifest struct {
		DisplayName *string `json:"displayName"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("sidecar.json: decode embedded manifest: %w", err)
	}
	if manifest.DisplayName == nil || *manifest.DisplayName == "" {
		return "", fmt.Errorf("sidecar.json: displayName is required")
	}
	return *manifest.DisplayName, nil
}
