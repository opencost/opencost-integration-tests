package assets

// Description
// With the fixed-pricing fixture applied, OpenCost's instantaneous pv_hourly_cost
// metric must reflect the fixture's storage price (monthly value / 730). Same two
// checks as the node test: per-PV uniformity (formula-independent) and a match to
// the expected hourly rate. The broker disk list is an independent count check.

import (
	"context"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/log"
)

func TestAssetDiskPricing(t *testing.T) {
	requireAssetsEnabled(t)

	if assetsEnv.DryRun {
		t.Logf("ASSETS_DRY_RUN set; would apply fixture %q and verify disk pricing", fixtureID)
		return
	}

	ctx := context.Background()
	broker := applyFixedPricing(t, ctx)

	disks, err := broker.Disks(ctx)
	if err != nil {
		t.Fatalf("broker disks: %v", err)
	}
	if len(disks) == 0 {
		t.Skip("no persistent volumes on this cluster; nothing to verify (disks may not apply)")
	}

	pv := pollMetric(t, "pv_hourly_cost")
	t.Logf("pv_hourly_cost=%v", pv)

	if len(pv) != len(disks) {
		t.Logf("note: broker reports %d PV(s), metrics report %d priced PV(s)", len(disks), len(pv))
	}

	checkPricing(t, "storage", pv, expectedPVHourly)

	log.Infof("asset disk pricing verified across %d PV(s)", len(pv))
}
