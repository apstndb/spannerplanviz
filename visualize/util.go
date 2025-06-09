package visualize

import (
	"fmt"
)

func markupIfNotEmpty(s, element string) string {
	if s == "" {
		return ""
	}

	return fmt.Sprintf("<%s>%s</%s>", element, s, element)
}
