package cmd

import (
	"fmt"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/resources/awsconnection"
	"github.com/dynatrace-oss/dtctl/pkg/resources/awsmonitoringconfig"
	"github.com/spf13/cobra"
)

type awsConnectionTableRow struct {
	Name       string `table:"NAME"`
	Type       string `table:"TYPE"`
	RoleArn    string `table:"ROLE_ARN"`
	ExternalID string `table:"EXTERNAL_ID"`
	ObjectID   string `table:"ID"`
}

func useAWSConnectionTableView() bool {
	return outputFormat == "" || outputFormat == "table" || outputFormat == "wide"
}

func toAWSConnectionTableRow(item *awsconnection.AWSConnection) awsConnectionTableRow {
	return awsConnectionTableRow{
		Name:       item.Name,
		Type:       item.Type,
		RoleArn:    item.RoleArn,
		ExternalID: item.ExternalID,
		ObjectID:   item.ObjectID,
	}
}

func toAWSConnectionTableRows(items []awsconnection.AWSConnection) []awsConnectionTableRow {
	rows := make([]awsConnectionTableRow, 0, len(items))
	for i := range items {
		rows = append(rows, toAWSConnectionTableRow(&items[i]))
	}
	return rows
}

var getAWSConnectionCmd = &cobra.Command{
	Use:     "connections [id]",
	Aliases: []string{"connection"},
	Short:   "Get AWS connections",
	Long:    `Get one or more AWS connections (authentication credentials).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := awsconnection.NewHandler(c)
		printer := NewPrinter()

		if len(args) > 0 {
			identifier := args[0]

			item, err := handler.FindByName(identifier)
			if err == nil {
				if useAWSConnectionTableView() {
					row := toAWSConnectionTableRow(item)
					return printer.Print(row)
				}
				return printer.Print(item)
			}

			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				item, err = handler.Get(identifier)
				if err != nil {
					return fmt.Errorf("connection with name or ID %q not found", identifier)
				}
				if useAWSConnectionTableView() {
					row := toAWSConnectionTableRow(item)
					return printer.Print(row)
				}
				return printer.Print(item)
			}
			return err
		}

		items, err := handler.List()
		if err != nil {
			return err
		}
		if useAWSConnectionTableView() {
			return printer.PrintList(toAWSConnectionTableRows(items))
		}
		return printer.PrintList(items)
	},
}

var getAWSMonitoringConfigCmd = &cobra.Command{
	Use:     "monitoring [id]",
	Aliases: []string{"monitoring-config", "monitoring-configs"},
	Short:   "Get AWS monitoring configurations",
	Long:    `Get one or more AWS monitoring configurations.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := awsmonitoringconfig.NewHandler(c)
		printer := NewPrinter()

		if len(args) > 0 {
			identifier := args[0]

			item, err := handler.FindByName(identifier)
			if err == nil {
				return printer.Print(item)
			}

			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				item, err = handler.Get(identifier)
				if err != nil {
					return fmt.Errorf("monitoring config with name/description or ID %q not found", identifier)
				}
				return printer.Print(item)
			}
			return err
		}

		items, err := handler.List()
		if err != nil {
			return err
		}
		return printer.PrintList(items)
	},
}

var getAWSMonitoringConfigRegionsCmd = &cobra.Command{
	Use:     "monitoring-locations",
	Aliases: []string{"monitoring-location"},
	Short:   "Get available AWS monitoring regions",
	Long:    `Get available AWS regions for monitoring configuration based on the latest extension schema.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := awsmonitoringconfig.NewHandler(c)
		printer := NewPrinter()

		regions, err := handler.ListAvailableRegions()
		if err != nil {
			return err
		}

		return printer.PrintList(regions)
	},
}

var getAWSMonitoringConfigFeatureSetsCmd = &cobra.Command{
	Use:     "monitoring-feature-sets",
	Aliases: []string{"monitoring-feature-set"},
	Short:   "Get available AWS monitoring feature sets",
	Long:    `Get available FeatureSetsType values for AWS monitoring configuration based on the latest extension schema.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := awsmonitoringconfig.NewHandler(c)
		printer := NewPrinter()

		featureSets, err := handler.ListAvailableFeatureSets()
		if err != nil {
			return err
		}

		return printer.PrintList(featureSets)
	},
}

func init() {
	getAWSProviderCmd.AddCommand(getAWSConnectionCmd)
	getAWSProviderCmd.AddCommand(getAWSMonitoringConfigCmd)
	getAWSProviderCmd.AddCommand(getAWSMonitoringConfigRegionsCmd)
	getAWSProviderCmd.AddCommand(getAWSMonitoringConfigFeatureSetsCmd)
}
