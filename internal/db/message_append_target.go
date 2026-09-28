package db

import (
	"errors"
	"reflect"
)

// currentAppendTarget uses the reconciled in-memory header on live writes.
// Recovery still calls prepareCLIReplyTarget, since an on-disk header may lag
// the transcript. Scanning IDs is cheap; decoding every historical tool trace
// for each append made a growing session quadratic in total trace bytes.
func currentAppendTarget(s Session, msgs []Message, m Message) (Session, bool, error) {
	for _, existing := range msgs {
		if existing.ID != m.ID {
			continue
		}
		if !reflect.DeepEqual(existing, m) {
			return Session{}, false, errors.New("message id collision")
		}
		return s, false, nil
	}
	s.MessageCount = len(msgs) + 1
	if m.Role == "assistant" {
		s.ToolCallCount += countToolSteps(m.Steps)
	}
	s.UpdatedAt = max(s.UpdatedAt, m.CreatedAt)
	s.Participants = addParticipant(s.Participants, m.AuthorKind, m.AuthorID)
	s.Participants = addParticipant(s.Participants, AuthorAgent, m.RecipientID)
	if m.Role == "assistant" && !IsMachineTranscriptKind(s.Kind) {
		s.Unread = true
	}
	return s, true, nil
}
