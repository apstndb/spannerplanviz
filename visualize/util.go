package visualize

func skipEmpty(values ...string) []string {
	var result []string
	for _, s := range values {
		if s == "" {
			continue
		}
		result = append(result, s)
	}
	return result
}

func in(s string, set ...string) bool {
	for _, elem := range set {
		if elem == s {
			return true
		}
	}
	return false
}

func encloseIfNotEmpty(open, s, close string) string {
	if s == "" {
		return ""
	}
	return open + s + close
}
