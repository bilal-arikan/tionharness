package decider

import (
	"errors"
	"strings"
	"testing"
)

func TestAuthorityRegistry(t *testing.T) {
	list := Authorities()
	if len(list) != 2 || list[0].ID != testGate || list[1].ID != testExplicit {
		t.Errorf("authorities = %+v, want the safety group before the flows group", list)
	}
	if a, ok := AuthorityByID(testExplicit); !ok || !a.Explicit || !a.FailClosed {
		t.Errorf("lookup = %+v, %v", a, ok)
	}
	bad := []Authority{
		{ID: "bad id", Group: GroupSafety, Modes: []Mode{ModeOn}, DefaultMode: ModeOn, DefaultThreshold: 0.7},
		{ID: "a1", Group: "misc", Modes: []Mode{ModeOn}, DefaultMode: ModeOn, DefaultThreshold: 0.7},
		{ID: "a2", Group: GroupSafety, Modes: []Mode{ModeOff}, DefaultMode: ModeOn, DefaultThreshold: 0.7},
		{ID: "a3", Group: GroupFlows, Modes: []Mode{ModeShadow, ModeOn}, DefaultMode: ModeOn, DefaultThreshold: 0.7, Explicit: true},
		{ID: "a4", Group: GroupSafety, Modes: []Mode{ModeOn}, DefaultMode: ModeOn, DefaultThreshold: 0.2},
		{ID: "a5", Group: GroupSafety, Modes: []Mode{"loud"}, DefaultMode: "loud", DefaultThreshold: 0.7},
	}
	for _, a := range bad {
		if err := a.check(); err == nil {
			t.Errorf("malformed authority accepted: %+v", a)
		}
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "registered twice") {
			t.Errorf("duplicate registration: recover = %v", r)
		}
	}()
	RegisterAuthority(Authority{ID: testGate, Group: GroupSafety, Modes: []Mode{ModeOn}, DefaultMode: ModeOn, DefaultThreshold: 0.7})
}

func TestValidateForLimits(t *testing.T) {
	req := Request{State: "s", Questions: map[string]Question{
		"q": Choice("?", map[string]string{"a": "A", "b": "B", "c": "C"}),
		"r": Score("?", "0", "1", "2", "3"),
	}}
	if err := req.ValidateFor(Limits{MaxOptions: 3, MaxLevels: 4}); err != nil {
		t.Errorf("request inside limits refused: %v", err)
	}
	err := req.ValidateFor(Limits{MaxOptions: 2})
	if err == nil || errors.Is(err, ErrInvalidRequest) {
		t.Errorf("err = %v; a limit error must not read as an invalid request (another model may accept it)", err)
	}
	if err := (Request{State: "s"}).ValidateFor(Limits{}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("err = %v, want ErrInvalidRequest", err)
	}
}
