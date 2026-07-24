package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/cost"
	"github.com/dynatrace-oss/dtctl/pkg/resources/bucket"
)

// costCmd groups cost-related helpers.
var costCmd = &cobra.Command{
	Use:   "cost",
	Short: "Estimate Dynatrace usage and cost",
	Long: `Estimate Dynatrace usage and cost from the current environment.

The estimator is heuristic-based: it uses bucket retention, record counts,
and estimated uncompressed bytes to approximate a monthly cost.`,
	RunE: requireSubcommand,
}

// costEstimateCmd estimates monthly bucket cost using the current Dynatrace context.
var costEstimateCmd = &cobra.Command{
	Use:   "estimate",
	Short: "Estimate monthly storage and usage cost",
	Long: `Estimate the current environment's cost based on bucket metadata.

The output includes a per-bucket breakdown and a total monthly estimate.

Use the pricing flags to override the default heuristic assumptions when you
want to model a specific contract or storage profile.`,
	RunE: runCostEstimate,
}

const (
	costStoragePriceEnv = "DTCTL_COST_STORAGE_PRICE_PER_GIB_MONTH"
	costRecordPriceEnv  = "DTCTL_COST_RECORD_PRICE_PER_MILLION"
	costBucketBaseEnv   = "DTCTL_COST_BUCKET_BASE_MONTHLY"
)

func costPricingFlags(cmd *cobra.Command) {
	cmd.Flags().Float64("storage-price-per-gib-month", 0, "storage price in USD per GiB-month")
	cmd.Flags().Float64("record-price-per-million", 0, "record price in USD per million records")
	cmd.Flags().Float64("bucket-base-monthly", 0, "base monthly fee per bucket in USD")
}

func runCostEstimate(cmd *cobra.Command, args []string) error {
	_, c, printer, err := Setup()
	if err != nil {
		return err
	}

	handler := bucket.NewHandler(c)
	list, err := handler.List()
	if err != nil {
		return err
	}

	result := cost.EstimateBuckets(list.Buckets, buildCostPricing(cmd))

	format := outputFormat
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if format == "yaml" {
		return printer.Print(result)
	}

	fmt.Printf("Estimated monthly cost: $%.2f\n", result.TotalMonthlyCostUSD)
	fmt.Printf("Buckets: %d\n", result.BucketCount)
	fmt.Printf("Total records: %d\n", result.TotalRecords)
	fmt.Printf("Estimated monthly GiB: %.3f\n", result.TotalEstimatedMonthlyGiB)
	fmt.Println()
	if len(result.Notes) > 0 {
		for _, note := range result.Notes {
			fmt.Printf("- %s\n", note)
		}
		fmt.Println()
	}
	if len(result.Buckets) > 0 {
		return printer.PrintList(result.Buckets)
	}
	fmt.Println("No buckets found in the current context.")
	return nil
}

func buildCostPricing(cmd *cobra.Command) cost.Pricing {
	pricing := cost.Pricing{}
	if v, ok, err := pricingValue(cmd, "storage-price-per-gib-month", costStoragePriceEnv); err != nil {
		return cost.Pricing{}
	} else if ok {
		pricing.StoragePricePerGiBMonth = v
	}
	if v, ok, err := pricingValue(cmd, "record-price-per-million", costRecordPriceEnv); err != nil {
		return cost.Pricing{}
	} else if ok {
		pricing.RecordPricePerMillion = v
	}
	if v, ok, err := pricingValue(cmd, "bucket-base-monthly", costBucketBaseEnv); err != nil {
		return cost.Pricing{}
	} else if ok {
		pricing.BucketBaseMonthly = v
	}
	return pricing.Normalize()
}

func pricingValue(cmd *cobra.Command, flagName, envName string) (float64, bool, error) {
	if cmd.Flags().Changed(flagName) {
		value, err := cmd.Flags().GetFloat64(flagName)
		return value, true, err
	}

	raw := os.Getenv(envName)
	if raw == "" {
		return 0, false, nil
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid %s value %q: %w", envName, raw, err)
	}
	return value, true, nil
}

func init() {
	costCmd.AddCommand(costEstimateCmd)
	rootCmd.AddCommand(costCmd)
	costPricingFlags(costEstimateCmd)
}
