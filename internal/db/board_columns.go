package db

// IsValidBoardKey reports whether s is safe to use as a kanban column key.
// Accepts lowercase letters, digits and underscores. All five built-in Board*
// constants satisfy this rule; custom column keys must follow the same pattern.
// Unlike ValidBoardState, this function is not restricted to the built-in set.
func IsValidBoardKey(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}
