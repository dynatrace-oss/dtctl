//go:build dqlbench
// +build dqlbench

package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/dqlcost"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
)

// TestFixtures_TenantBench executes each fixture against a live Dynatrace
// tenant, records ScannedBytes for both the raw query and the cost-rewritten
// query, and writes a markdown comparison report under ../reports/.
//
// Gated behind build tag `dqlbench` so normal unit runs don't hit the API.
func TestFixtures_TenantBench(t *testing.T) {
	envURL := os.Getenv("DTCTL_INTEGRATION_ENV")
	token := os.Getenv("DTCTL_INTEGRATION_TOKEN")
	if envURL == "" || token == "" {
		t.Skip("DTCTL_INTEGRATION_ENV / DTCTL_INTEGRATION_TOKEN not set")
	}

	fixtures, err := LoadFixtures(fixturesDir(t))
	if err != nil {
		t.Fatalf("LoadFixtures: %v", err)
	}

	c, err := client.New(envURL, token)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	ex := exec.NewDQLExecutor(c)

	reportDir, err := filepath.Abs("../reports")
	if err != nil {
		t.Fatalf("resolve reports dir: %v", err)
	}
	_ = os.MkdirAll(reportDir, 0o755)
	reportPath := filepath.Join(reportDir, fmt.Sprintf("%s.md", time.Now().UTC().Format("20060102T150405Z")))

	f, err := os.Create(reportPath)
	if err != nil {
		t.Fatalf("create report: %v", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "# dqlbench report — %s\n\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(f, "| fixture | raw scanned bytes | rewritten scanned bytes | Δ%% | budget ok |\n")
	fmt.Fprintf(f, "|---------|-------------------|-------------------------|----|-----------|\n")

	opts := exec.DQLExecuteOptions{
		IncludeContributions: true,
		MetadataFields:       []string{"all"},
		MaxResultRecords:     100,
	}

	for _, fx := range fixtures {
		t.Run(fx.ID, func(t *testing.T) {
			rawBytes, _ := runOne(ex, fx.Query, opts)
			rewritten, _ := dqlcost.Rewrite(fx.Query, dqlcost.DefaultRewriteOptions())
			rewrittenBytes, _ := runOne(ex, rewritten, opts)

			delta := "n/a"
			if rawBytes > 0 {
				delta = fmt.Sprintf("%.1f%%", 100.0*float64(rewrittenBytes-rawBytes)/float64(rawBytes))
			}
			budgetOK := "n/a"
			if fx.Budget.MaxScannedBytes > 0 {
				if rewrittenBytes <= fx.Budget.MaxScannedBytes {
					budgetOK = "yes"
				} else {
					budgetOK = "no"
					t.Errorf("budget exceeded: rewritten scan %d > budget %d", rewrittenBytes, fx.Budget.MaxScannedBytes)
				}
			}
			fmt.Fprintf(f, "| %s | %d | %d | %s | %s |\n", fx.ID, rawBytes, rewrittenBytes, delta, budgetOK)
		})
	}

	t.Logf("report written to %s", reportPath)
}

func runOne(ex *exec.DQLExecutor, query string, opts exec.DQLExecuteOptions) (int64, error) {
	resp, err := ex.ExecuteQueryWithOptions(query, opts)
	if err != nil || resp == nil || resp.Result == nil || resp.Result.Metadata == nil || resp.Result.Metadata.Grail == nil {
		return 0, err
	}
	return resp.Result.Metadata.Grail.ScannedBytes, nil
}
