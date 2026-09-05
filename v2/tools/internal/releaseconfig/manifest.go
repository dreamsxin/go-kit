// Package releaseconfig loads the v2 release manifest.
package releaseconfig

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest describes the release being cut.
//
// v2 ships as one module, so a release is one commit and one tag. Phase is
// "candidate" until the release is recorded and "released" afterwards. Only the
// candidate phase constrains the tag — it must not exist yet, or the release
// would be cut against a version that is already published and immutable. The
// released phase says nothing about the local tag, because recording the release
// is itself a commit that the release checks run against.
type Manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	Phase         string `json:"phase"`
	ReleaseDate   string `json:"releaseDate,omitempty"`
	CoreVersion   string `json:"coreVersion"`
	Tag           string `json:"tag"`
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
