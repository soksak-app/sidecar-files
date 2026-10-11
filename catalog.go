// Package catalog holds the event catalog that sidecar.json names in `events`. The sidecar reads the class of each event from it,
// so the catalog is the one owner of the classes that the policy gates.
package catalog

import _ "embed"

// Events is the text of events.json.
//
//go:embed events.json
var Events []byte
