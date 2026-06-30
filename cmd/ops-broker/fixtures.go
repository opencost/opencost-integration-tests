package main

import (
	"embed"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// fixtureOwnerLabel marks ConfigMaps the broker applied, so cleanup only ever
// deletes broker-owned objects — never an arbitrary caller-named resource.
const fixtureOwnerLabel = "opencost.io/fixture"

// fixtureSnapshotAnnotation stores the JSON-encoded original ConfigMap state
// (data/labels/annotations) the fixture overwrote, so cleanup can restore it
// exactly. Absent on a broker-owned ConfigMap means the fixture created it and
// cleanup should delete it.
const fixtureSnapshotAnnotation = "opencost.io/fixture-snapshot"

//go:embed fixtures/*.yaml
var fixtureFS embed.FS

// fixtureSpec describes one allowlisted fixture: its ID and the embedded
// manifest baked into the broker image. The caller only ever names the ID; the
// manifest never crosses the trust boundary.
type fixtureSpec struct {
	id   string
	file string
}

// allowedFixtures is the frozen allowlist of fixture IDs the broker will apply.
// Cloud-cost fixtures were dropped; only the asset pricing fixture remains.
var allowedFixtures = map[string]fixtureSpec{
	"pricing-fixed-v1": {id: "pricing-fixed-v1", file: "fixtures/pricing-fixed-v1.yaml"},
}

// loadFixtureConfigMap parses the embedded manifest for an allowlisted fixture
// into a ConfigMap and stamps the broker-owned label. Unknown IDs return
// notAllowedError so handlers map them to 404.
func loadFixtureConfigMap(fixtureID string) (*corev1.ConfigMap, error) {
	spec, ok := allowedFixtures[fixtureID]
	if !ok {
		return nil, notAllowedError(fmt.Sprintf("unknown fixture %q", fixtureID))
	}

	raw, err := fixtureFS.ReadFile(spec.file)
	if err != nil {
		return nil, fmt.Errorf("reading embedded fixture %q: %w", fixtureID, err)
	}

	cm := &corev1.ConfigMap{}
	if err := yaml.Unmarshal(raw, cm); err != nil {
		return nil, fmt.Errorf("parsing embedded fixture %q: %w", fixtureID, err)
	}
	if cm.Labels == nil {
		cm.Labels = map[string]string{}
	}
	cm.Labels[fixtureOwnerLabel] = fixtureID
	return cm, nil
}
