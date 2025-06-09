package visualize

import (
	"fmt"
)

func markupIfNotEmpty(element, s string) string {
	if s == "" {
		return ""
	}

	return fmt.Sprintf("<%s>%s</%s>", element, s, element)
}
