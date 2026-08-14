package releaseconfig

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest defines the versions, tags, and order of a multi-module release.
type Manifest struct {
	SchemaVersion       int      `json:"schemaVersion"`
	Phase               string   `json:"phase"`
	ReleaseDate         string   `json:"releaseDate,omitempty"`
	PreviousCoreVersion string   `json:"previousCoreVersion"`
	CoreVersion         string   `json:"coreVersion"`
	Modules             []Module `json:"modules"`
}

// Module is one independently versioned module in a release.
type Module struct {
	Directory     string `json:"directory"`
	ModulePath    string `json:"modulePath"`
	Version       string `json:"version"`
	Tag           string `json:"tag"`
	ReleaseOrder  int    `json:"releaseOrder"`
	DependsOnCore bool   `json:"dependsOnCore"`
}

// LoadManifest reads and decodes a release manifest.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read release manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	return manifest, nil
}
