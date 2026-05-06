package cmdutil

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func IsHelp(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

func SplitInlineFlag(arg string) (name, value string, ok bool) {
	if !strings.HasPrefix(arg, "--") {
		return arg, "", false
	}
	name, value, ok = strings.Cut(arg, "=")
	return name, value, ok
}

func OptionValue(args []string, index int, flag, inlineValue string, hasInlineValue bool) (string, int, error) {
	if hasInlineValue {
		if strings.TrimSpace(inlineValue) == "" {
			return "", index, fmt.Errorf("%s requires a non-empty value", flag)
		}
		return inlineValue, index, nil
	}
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("%s requires a value", flag)
	}
	value := args[index+1]
	if strings.TrimSpace(value) == "" {
		return "", index, fmt.Errorf("%s requires a non-empty value", flag)
	}
	return value, index + 1, nil
}

func NonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func WriteJSON(out io.Writer, payload any) error {
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", encoded)
	return err
}
