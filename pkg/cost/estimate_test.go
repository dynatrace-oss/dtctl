package cost

import (
	"math"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
)

func TestEstimateBuckets(t *testing.T) {
	bytes1 := int64(1024 * 1024 * 1024)
	records1 := int64(1_000_000)

	result := EstimateBuckets([]bucket.Bucket{
		{
			BucketName:                 "alpha",
			Table:                      "logs",
			RetentionDays:              30,
			Records:                    &records1,
			EstimatedUncompressedBytes: &bytes1,
		},
		{
			BucketName:    "beta",
			Table:         "metrics",
			RetentionDays: 60,
		},
	}, Pricing{
		StoragePricePerGiBMonth: 1,
		RecordPricePerMillion:   2,
		BucketBaseMonthly:       3,
	})

	if result.BucketCount != 2 {
		t.Fatalf("BucketCount = %d, want 2", result.BucketCount)
	}
	if result.Buckets[0].BucketName != "alpha" {
		t.Fatalf("expected highest-cost bucket first, got %q", result.Buckets[0].BucketName)
	}
	if math.Abs(result.TotalMonthlyCostUSD-9) > 0.001 {
		t.Fatalf("TotalMonthlyCostUSD = %.3f, want 9.000", result.TotalMonthlyCostUSD)
	}
	if math.Abs(result.Buckets[0].MonthlyCostUSD-6) > 0.001 {
		t.Fatalf("alpha MonthlyCostUSD = %.3f, want 6.000", result.Buckets[0].MonthlyCostUSD)
	}
	if math.Abs(result.Buckets[1].MonthlyCostUSD-3) > 0.001 {
		t.Fatalf("beta MonthlyCostUSD = %.3f, want 3.000", result.Buckets[1].MonthlyCostUSD)
	}
}
