package catalog

import "testing"

func TestProcessDisplayNameUsesTheEmbeddedManifest(t *testing.T) {
	name, err := ProcessDisplayName()
	if err != nil {
		t.Fatal(err)
	}
	if name != "soksak://sidecar/files" {
		t.Fatalf("process display name = %q, want the sidecar manifest identity", name)
	}
}
