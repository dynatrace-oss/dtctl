package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/resources/database"
)

var getDatabasesCmd = &cobra.Command{
	Use:     "databases",
	Aliases: []string{"database", "db-instances"},
	Short:   "Get database instances",
	Long: `List database instances monitored by Dynatrace via Smartscape.

Queries native Dynatrace database node types:
  DB_INSTANCE_POSTGRES  PostgreSQL instances
  DB_INSTANCE_MYSQL     MySQL instances
  DB_INSTANCE_MSSQL     Microsoft SQL Server instances
  DB_INSTANCE_MARIADB   MariaDB instances

Results from all types are combined and sorted by name.
Use --vendor to restrict results to a specific database engine.`,
	Example: `  # List all monitored database instances
  dtctl get databases

  # Filter to PostgreSQL only
  dtctl get databases --vendor postgres

  # JSON output
  dtctl get databases -o json

  # Wide output (includes type, host, port columns)
  dtctl get databases -o wide`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, c, printer, err := Setup()
		if err != nil {
			return err
		}

		vendor, _ := cmd.Flags().GetString("vendor")

		handler := database.NewHandler(c)
		list, err := handler.List(database.ListOptions{Vendor: vendor})
		if err != nil {
			return err
		}

		return printer.PrintList(list.Databases)
	},
}

func init() {
	getDatabasesCmd.Flags().String("vendor", "", "Filter by database vendor (postgres, mysql, mssql, mariadb)")
}
