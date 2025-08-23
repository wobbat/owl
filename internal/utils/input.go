package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// GetUserConfirmation prompts the user for confirmation and returns their response
func GetUserConfirmation(prompt string) (string, error) {
	fmt.Print(prompt)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(input), nil
}

// ConfirmAction asks the user to confirm an action with a yes/no prompt
func ConfirmAction(message string) (bool, error) {
	response, err := GetUserConfirmation(fmt.Sprintf("%s (y/N): ", message))
	if err != nil {
		return false, err
	}

	response = strings.ToLower(response)
	return response == "y" || response == "yes", nil
}
