package utils

// IsStringInStringSlice accepts a search string and a slice of strings.
// It returns true if the search string is found in the slice, otherwise false.
func IsStringInStringSlice(s string, sl []string) bool {
	for _, item := range sl {
		if item == s {
			return true
		}
	}

	return false
}
