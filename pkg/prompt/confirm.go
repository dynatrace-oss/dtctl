package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Confirm prompts the user for yes/no confirmation
// Returns true if user confirms, false otherwise
func Confirm(message string) bool {
	fmt.Printf("%s [y/N]: ", message)

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

// ConfirmDeletion prompts for confirmation of a destructive operation
// Shows resource details and requires explicit confirmation
func ConfirmDeletion(resourceType, name, id string) bool {
	fmt.Printf("\nYou are about to delete the following %s:\n", resourceType)
	fmt.Printf("  Name: %s\n", name)
	fmt.Printf("  ID:   %s\n", id)
	fmt.Println()

	return Confirm("Are you sure you want to delete this resource?")
}
