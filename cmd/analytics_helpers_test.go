package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestBuildCostPricingOverridesDefaults(t *testing.T) {
	cmd := &cobra.Command{Use: "estimate"}
	costPricingFlags(cmd)
	require.NoError(t, cmd.Flags().Set("storage-price-per-gib-month", "1.25"))
	require.NoError(t, cmd.Flags().Set("record-price-per-million", "2.5"))
	require.NoError(t, cmd.Flags().Set("bucket-base-monthly", "3.75"))

	pricing := buildCostPricing(cmd)
	require.InDelta(t, 1.25, pricing.StoragePricePerGiBMonth, 0.0001)
	require.InDelta(t, 2.5, pricing.RecordPricePerMillion, 0.0001)
	require.InDelta(t, 3.75, pricing.BucketBaseMonthly, 0.0001)
}

func TestBuildCostPricingUsesEnvironmentFallback(t *testing.T) {
	t.Setenv(costStoragePriceEnv, "1.5")
	t.Setenv(costRecordPriceEnv, "2.25")
	t.Setenv(costBucketBaseEnv, "4.75")

	cmd := &cobra.Command{Use: "estimate"}
	costPricingFlags(cmd)

	pricing := buildCostPricing(cmd)
	require.InDelta(t, 1.5, pricing.StoragePricePerGiBMonth, 0.0001)
	require.InDelta(t, 2.25, pricing.RecordPricePerMillion, 0.0001)
	require.InDelta(t, 4.75, pricing.BucketBaseMonthly, 0.0001)
}

func TestAnalyzePresetCommandsRegistered(t *testing.T) {
	for _, name := range []string{"forecast", "anomaly", "change-point", "correlation"} {
		cmd, _, err := rootCmd.Find([]string{"analyze", name})
		require.NoError(t, err)
		require.NotNil(t, cmd)
		require.Equal(t, name, cmd.Name())
	}
}
