package assets

import (
	"math"
	"testing"
)

func TestParseMetricValues(t *testing.T) {
	body := `# HELP node_cpu_hourly_cost The hourly cost per CPU
# TYPE node_cpu_hourly_cost gauge
node_cpu_hourly_cost{node="a"} 0.00137
node_cpu_hourly_cost{node="b"} 0.00137
node_ram_hourly_cost{node="a"} 0.000685
pv_hourly_cost{persistentvolume="p1"} 5.5e-05
node_cpu_hourly_cost_total{node="a"} 9.9
`

	cpu := parseMetricValues(body, "node_cpu_hourly_cost")
	if len(cpu) != 2 || cpu[0] != 0.00137 || cpu[1] != 0.00137 {
		t.Fatalf("cpu = %v, want [0.00137 0.00137] (and no prefix collision)", cpu)
	}
	ram := parseMetricValues(body, "node_ram_hourly_cost")
	if len(ram) != 1 || ram[0] != 0.000685 {
		t.Fatalf("ram = %v, want [0.000685]", ram)
	}
	pv := parseMetricValues(body, "pv_hourly_cost")
	if len(pv) != 1 || pv[0] != 5.5e-05 {
		t.Fatalf("pv = %v, want [5.5e-05]", pv)
	}
	if got := parseMetricValues(body, "node_gpu_hourly_cost"); len(got) != 0 {
		t.Fatalf("absent metric = %v, want empty", got)
	}
}

// Documents the monthly→hourly conversion the expected-price constants encode.
func TestExpectedHourlyFromMonthly(t *testing.T) {
	cases := []struct {
		name    string
		got     float64
		monthly float64
	}{
		{"cpu", expectedNodeCPUHourly, fixtureCPUMonthly},
		{"ram", expectedNodeRAMHourly, fixtureRAMMonthly},
		{"pv", expectedPVHourly, fixtureStorageMonthly},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.monthly / hoursPerMonth
			if math.Abs(tc.got-want) > 1e-12 {
				t.Fatalf("%s expected hourly = %v, want %v", tc.name, tc.got, want)
			}
		})
	}
}
