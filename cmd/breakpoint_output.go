package cmd

type breakpointCommandOutput struct {
	Message string `json:"message" yaml:"message" table:"MESSAGE"`
}

func printBreakpointMessage(verb, message string) error {
	printer := NewPrinter()
	_ = enrichAgent(printer, verb, "breakpoint")
	return printer.Print(breakpointCommandOutput{Message: message})
}
