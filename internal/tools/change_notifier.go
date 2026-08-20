package tools

import (
	"sync"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// ChangeNotifier is the narrow seam used by self-management tools to notify
// application surfaces after a successful mutation. A nil notifier keeps tools
// usable in tests and standalone contexts.
type ChangeNotifier interface {
	BoardChanged(database *db.DB, change BoardChange)
	AgentModelChanged(database *db.DB, change AgentModelChange)
}

type BoardChange struct {
	Title  string
	Body   string
	TaskID string
	Op     string
}

type AgentModelChange struct {
	AgentID   string
	AgentName string
	OldModel  string
	NewModel  string
}

var changeNotifier struct {
	sync.RWMutex
	value ChangeNotifier
}

// SetChangeNotifier installs the process-wide application adapter.
func SetChangeNotifier(notifier ChangeNotifier) {
	changeNotifier.Lock()
	changeNotifier.value = notifier
	changeNotifier.Unlock()
}

func notifyBoardChanged(database *db.DB, change BoardChange) {
	changeNotifier.RLock()
	notifier := changeNotifier.value
	changeNotifier.RUnlock()
	if notifier != nil {
		notifier.BoardChanged(database, change)
	}
}

func notifyAgentModelChanged(database *db.DB, change AgentModelChange) {
	changeNotifier.RLock()
	notifier := changeNotifier.value
	changeNotifier.RUnlock()
	if notifier != nil {
		notifier.AgentModelChanged(database, change)
	}
}

// AgentModelChangeWarning returns the response warning shared by REST and tool
// updates when the selected model has neither exact nor estimated pricing.
func AgentModelChangeWarning(agent db.Agent) string {
	_, pricedOK := providers.PriceFor(agent.Provider, agent.Model)
	_, estimatedOK := providers.EstimateFor(agent.Provider, agent.Model)
	if pricedOK || estimatedOK {
		return ""
	}
	return "Bu model (" + agent.Model + ") fiyat tablosunda bulunamadı — bütçe kayıtları 'fiyatlandırılmamış' görünebilir."
}
