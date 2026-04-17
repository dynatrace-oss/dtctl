package cmd

func init() {
	enableCmd.AddCommand(enableGCPProviderCmd)
	attachPreviewNotice(enableGCPProviderCmd, "GCP")
}
