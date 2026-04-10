package cmd

func init() {
	activateCmd.AddCommand(activateGCPProviderCmd)
	attachPreviewNotice(activateGCPProviderCmd, "GCP")
}
