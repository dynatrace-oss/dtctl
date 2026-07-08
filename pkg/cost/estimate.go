package cost

import (
	"math"
	"sort"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
)

const (
	defaultStoragePricePerGiBMonth = 0.08
	defaultRecordPricePerMillion   = 0.12
	defaultBucketBaseMonthly       = 0.50
	defaultDaysPerMonth            = 30.0
	defaultBytesPerRecord          = 4 * 1024
	bytesPerGiB                    = 1024 * 1024 * 1024
)

// Pricing captures the pricing assumptions used by the estimator.
type Pricing struct {
	StoragePricePerGiBMonth float64 `json:"storagePricePerGiBMonth"`
	RecordPricePerMillion   float64 `json:"recordPricePerMillion"`
	BucketBaseMonthly       float64 `json:"bucketBaseMonthly"`
}

// Normalize returns a pricing model with sensible defaults for unset fields.
func (p Pricing) Normalize() Pricing {
	if p.StoragePricePerGiBMonth <= 0 {
		p.StoragePricePerGiBMonth = defaultStoragePricePerGiBMonth
	}
	if p.RecordPricePerMillion <= 0 {
		p.RecordPricePerMillion = defaultRecordPricePerMillion
	}
	if p.BucketBaseMonthly <= 0 {
		p.BucketBaseMonthly = defaultBucketBaseMonthly
	}
	return p
}

// BucketEstimate describes the estimated monthly cost for a single bucket.
type BucketEstimate struct {
	BucketName          string  `json:"bucketName"`
	Table               string  `json:"table"`
	Records             int64   `json:"records"`
	EstimatedBytes      int64   `json:"estimatedBytes"`
	EstimatedMonthlyGiB float64 `json:"estimatedMonthlyGiB"`
	MonthlyCostUSD      float64 `json:"monthlyCostUsd"`
}

// EstimateResult is the full estimation report.
type EstimateResult struct {
	GeneratedAt              time.Time        `json:"generatedAt"`
	BucketCount              int              `json:"bucketCount"`
	TotalRecords             int64            `json:"totalRecords"`
	TotalEstimatedBytes      int64            `json:"totalEstimatedBytes"`
	TotalEstimatedMonthlyGiB float64          `json:"totalEstimatedMonthlyGiB"`
	TotalMonthlyCostUSD      float64          `json:"totalMonthlyCostUsd"`
	Pricing                  Pricing          `json:"pricing"`
	Buckets                  []BucketEstimate `json:"buckets"`
	Notes                    []string         `json:"notes,omitempty"`
}

// EstimateBuckets calculates a heuristic monthly cost estimate for the given buckets.
// The estimator prefers the API-provided estimated size and record count. If those
// are missing, it falls back to a conservative 4 KiB-per-record approximation.
func EstimateBuckets(buckets []bucket.Bucket, pricing Pricing) EstimateResult {
	pricing = pricing.Normalize()

	result := EstimateResult{
		GeneratedAt: time.Now().UTC(),
		Pricing:     pricing,
		Notes: []string{
			"This is a heuristic estimate based on bucket size and retention only.",
			"Dynatrace billing may differ depending on your contract and enabled products.",
		},
	}

	result.Buckets = make([]BucketEstimate, 0, len(buckets))
	for _, b := range buckets {
		estimate := estimateBucket(b, pricing)
		result.Buckets = append(result.Buckets, estimate)
		result.BucketCount++
		result.TotalRecords += estimate.Records
		result.TotalEstimatedBytes += estimate.EstimatedBytes
		result.TotalEstimatedMonthlyGiB += estimate.EstimatedMonthlyGiB
		result.TotalMonthlyCostUSD += estimate.MonthlyCostUSD
	}

	sort.Slice(result.Buckets, func(i, j int) bool {
		if result.Buckets[i].MonthlyCostUSD == result.Buckets[j].MonthlyCostUSD {
			return result.Buckets[i].BucketName < result.Buckets[j].BucketName
		}
		return result.Buckets[i].MonthlyCostUSD > result.Buckets[j].MonthlyCostUSD
	})

	return result
}

// EstimateDefaultBuckets uses the default pricing assumptions.
func EstimateDefaultBuckets(buckets []bucket.Bucket) EstimateResult {
	return EstimateBuckets(buckets, Pricing{})
}

func estimateBucket(b bucket.Bucket, pricing Pricing) BucketEstimate {
	retentionDays := b.RetentionDays
	if retentionDays <= 0 {
		retentionDays = int(defaultDaysPerMonth)
	}

	var estimatedBytes int64
	switch {
	case b.EstimatedUncompressedBytes != nil && *b.EstimatedUncompressedBytes > 0:
		estimatedBytes = *b.EstimatedUncompressedBytes
	case b.Records != nil && *b.Records > 0:
		estimatedBytes = *b.Records * defaultBytesPerRecord
	}

	var records int64
	switch {
	case b.Records != nil && *b.Records > 0:
		records = *b.Records
	case estimatedBytes > 0:
		records = int64(math.Round(float64(estimatedBytes) / defaultBytesPerRecord))
	}

	estimatedMonthlyBytes := float64(estimatedBytes) * float64(retentionDays) / defaultDaysPerMonth
	estimatedMonthlyGiB := estimatedMonthlyBytes / bytesPerGiB

	monthlyCost := pricing.BucketBaseMonthly
	monthlyCost += estimatedMonthlyGiB * pricing.StoragePricePerGiBMonth
	monthlyCost += (float64(records) / 1_000_000.0) * pricing.RecordPricePerMillion

	return BucketEstimate{
		BucketName:          b.BucketName,
		Table:               b.Table,
		Records:             records,
		EstimatedBytes:      estimatedBytes,
		EstimatedMonthlyGiB: estimatedMonthlyGiB,
		MonthlyCostUSD:      monthlyCost,
	}
}
