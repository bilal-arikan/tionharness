package db

// IsValidBoardKey reports whether s is safe to use as a kanban column key.
// Accepts lowercase letters, digits and underscores. All six built-in Board*
// constants satisfy this rule; custom column keys must follow the same pattern.
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

// countReviewBounce applies the failed-verification-round rule to a stored task
// moving from one column to another.
//
// It lives here, called from EVERY write path that can change a board state,
// because there is more than one: move_task goes through MoveTask, while the
// HTTP PUT behind a card drag (and the task form) goes through UpdateTask.
// Counting in only one of them made the badge invisible to exactly the path a
// human uses.
func countReviewBounce(t *Task, from, to string) {
	if from == BoardReview && isWorkingBoardState(to) {
		t.ReviewBounces++
	}
}

// isWorkingBoardState reports whether a column means "still being worked on" —
// the destinations a card falls back to when a verification round rejects it.
// Terminal columns (done/failed/cancelled) and review itself are excluded: a
// card moved from review to done passed, and review→review is not a round.
func isWorkingBoardState(key string) bool {
	switch key {
	case BoardPBI, BoardTodo, BoardInProgress:
		return true
	default:
		return false
	}
}
