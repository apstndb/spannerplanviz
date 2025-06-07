package visualize

import (
	"fmt"
	"slices"
)

func in(s string, set ...string) bool {
	return slices.Contains(set, s)
}

func markupIfNotEmpty(s, element string) string {
	if s == "" {
		return ""
	}

	return fmt.Sprintf("<%s>%s</%s>", element, s, element)
}
