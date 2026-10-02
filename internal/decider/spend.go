package decider

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const spendFileName = "spend.json"

// SpendTotals measures real backend calls, including failures and comparisons.
// UnknownCostCalls distinguishes missing prices from a reported zero price.
type SpendTotals struct {
	Calls            int     `json:"calls"`
	Failures         int     `json:"failures"`
	InputTokens      int     `json:"inputTokens"`
	OutputTokens     int     `json:"outputTokens"`
	UnknownCostCalls int     `json:"unknownCostCalls"`
	PrimaryCalls     int     `json:"primaryCalls"`
	ChallengerCalls  int     `json:"challengerCalls"`
	TestCalls        int     `json:"testCalls"`
	CostUSD          float64 `json:"costUSD"`
	ReportedCostUSD  float64 `json:"reportedCostUSD"`
	EstimatedCostUSD float64 `json:"estimatedCostUSD"`
}

type SpendModel struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	SpendTotals
}

type SpendReport struct {
	Days         int          `json:"days"`
	StartedAt    int64        `json:"startedAt"`
	UpdatedAt    int64        `json:"updatedAt"`
	StorageError bool         `json:"storageError"`
	Totals       SpendTotals  `json:"totals"`
	Today        SpendTotals  `json:"today"`
	Models       []SpendModel `json:"models"`
}

type spendDay struct {
	Day      string `json:"day"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	SpendTotals
}

type spendFile struct {
	Version   int        `json:"version"`
	StartedAt int64      `json:"startedAt"`
	UpdatedAt int64      `json:"updatedAt"`
	Days      []spendDay `json:"days"`
}

// spendStore is independent of the rotating decision/debug ledgers. Its only
// persisted data is daily numeric counters and billing model identifiers.
type spendStore struct {
	mu           sync.Mutex
	path         string
	logger       *slog.Logger
	startedAt    int64
	updatedAt    int64
	days         map[string]spendDay
	storageError bool
	loadError    error // unreadable history must never be silently overwritten
}

func openSpendStore(dir string, logger *slog.Logger, now time.Time) *spendStore {
	if logger == nil {
		logger = slog.Default()
	}
	s := &spendStore{logger: logger, startedAt: now.UnixMilli(), updatedAt: now.UnixMilli(), days: map[string]spendDay{}}
	if dir == "" {
		return s
	}
	s.path = filepath.Join(dir, spendFileName)
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		if err := s.persistLocked(); err != nil {
			s.storageError = true
			logger.Warn("decision spend history could not be initialized", "error", err)
		}
		return s
	}
	if err == nil {
		var file spendFile
		err = json.Unmarshal(data, &file)
		if err == nil {
			err = validateSpendFile(file)
		}
		if err == nil {
			s.startedAt, s.updatedAt = file.StartedAt, file.UpdatedAt
			for _, day := range file.Days {
				s.days[spendDayKey(day.Day, day.Provider, day.Model)] = day
			}
			before := len(s.days)
			s.trimLocked(now)
			if len(s.days) != before {
				if err := s.persistLocked(); err != nil {
					s.storageError = true
					logger.Warn("expired decision spend history could not be pruned", "error", err)
				}
			}
			return s
		}
	}
	s.storageError = true
	s.loadError = fmt.Errorf("load decision spend history: %w", err)
	logger.Warn("decision spend history unreadable; preserving stored file", "error", s.loadError)
	return s
}

// record counts exactly one completed backend attempt. The caller supplies
// billing identity even for a failed attempt that returned no Response.
func (s *spendStore) record(now time.Time, authority, role, provider, model string, usage Usage, failed bool) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	provider, model = spendIdentity(provider), spendIdentity(model)
	day := now.UTC().Format(time.DateOnly)
	key := spendDayKey(day, provider, model)
	row := s.days[key]
	row.Day, row.Provider, row.Model = day, provider, model
	row.Calls++
	if failed {
		row.Failures++
	}
	row.InputTokens += max(usage.InputTokens, 0)
	row.OutputTokens += max(usage.OutputTokens, 0)
	switch role {
	case "challenger":
		row.ChallengerCalls++
	case "test":
		row.TestCalls++
	default:
		row.PrimaryCalls++
	}
	if math.IsNaN(usage.CostUSD) || math.IsInf(usage.CostUSD, 0) || usage.CostUSD < 0 {
		row.UnknownCostCalls++
	} else {
		switch usage.CostSource {
		case "reported":
			row.CostUSD += usage.CostUSD
			row.ReportedCostUSD += usage.CostUSD
		case "estimated":
			row.CostUSD += usage.CostUSD
			row.EstimatedCostUSD += usage.CostUSD
		case "local":
			row.CostUSD += usage.CostUSD
		default:
			row.UnknownCostCalls++
		}
	}
	s.days[key] = row
	s.updatedAt = max(s.updatedAt, now.UnixMilli())
	s.trimLocked(now)
	// Still count current-process calls, but flag the report as incomplete and
	// preserve the original unreadable file for recovery instead of replacing it.
	if s.loadError != nil {
		return s.loadError
	}
	if err := s.persistLocked(); err != nil {
		s.storageError = true
		s.logger.Warn("decision spend history could not be persisted", "authority", authority, "error", err)
		return err
	}
	s.storageError = false
	return nil
}

// report uses inclusive UTC calendar days. Ninety days is the public report
// window; the underlying store retains 365 days and does not depend on ledger caps.
func (s *spendStore) report(now time.Time, days int) SpendReport {
	if days <= 0 {
		days = 30
	}
	days = min(days, 90)
	report := SpendReport{Days: days, Models: []SpendModel{}}
	if s == nil {
		return report
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	report.StartedAt, report.UpdatedAt, report.StorageError = s.startedAt, s.updatedAt, s.storageError
	today := utcSpendDay(now)
	start := today.AddDate(0, 0, -(days - 1)).Format(time.DateOnly)
	end := today.Format(time.DateOnly)
	models := map[string]SpendModel{}
	for _, row := range s.days {
		if row.Day < start || row.Day > end {
			continue
		}
		addSpendTotals(&report.Totals, row.SpendTotals)
		if row.Day == end {
			addSpendTotals(&report.Today, row.SpendTotals)
		}
		key := spendDayKey("", row.Provider, row.Model)
		model := models[key]
		model.Provider, model.Model = row.Provider, row.Model
		addSpendTotals(&model.SpendTotals, row.SpendTotals)
		models[key] = model
	}
	for _, model := range models {
		report.Models = append(report.Models, model)
	}
	sort.Slice(report.Models, func(i, j int) bool {
		a, b := report.Models[i], report.Models[j]
		if a.CostUSD != b.CostUSD {
			return a.CostUSD > b.CostUSD
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		return a.Model < b.Model
	})
	return report
}

func (s *spendStore) trimLocked(now time.Time) {
	cutoff := utcSpendDay(now).AddDate(0, 0, -364).Format(time.DateOnly)
	for key, day := range s.days {
		if day.Day < cutoff {
			delete(s.days, key)
		}
	}
}

func (s *spendStore) persistLocked() error {
	if s.path == "" {
		return nil
	}
	file := spendFile{Version: 1, StartedAt: s.startedAt, UpdatedAt: s.updatedAt, Days: make([]spendDay, 0, len(s.days))}
	for _, row := range s.days {
		file.Days = append(file.Days, row)
	}
	sort.Slice(file.Days, func(i, j int) bool {
		a, b := file.Days[i], file.Days[j]
		return spendDayKey(a.Day, a.Provider, a.Model) < spendDayKey(b.Day, b.Provider, b.Model)
	})
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, data)
}

func addSpendTotals(total *SpendTotals, delta SpendTotals) {
	total.Calls += delta.Calls
	total.Failures += delta.Failures
	total.InputTokens += delta.InputTokens
	total.OutputTokens += delta.OutputTokens
	total.UnknownCostCalls += delta.UnknownCostCalls
	total.PrimaryCalls += delta.PrimaryCalls
	total.ChallengerCalls += delta.ChallengerCalls
	total.TestCalls += delta.TestCalls
	total.CostUSD += delta.CostUSD
	total.ReportedCostUSD += delta.ReportedCostUSD
	total.EstimatedCostUSD += delta.EstimatedCostUSD
}

func validateSpendFile(file spendFile) error {
	if file.Version != 1 || file.StartedAt <= 0 || file.UpdatedAt < file.StartedAt {
		return errors.New("invalid spend history version or timestamps")
	}
	seen := map[string]bool{}
	for _, row := range file.Days {
		if _, err := time.Parse(time.DateOnly, row.Day); err != nil {
			return errors.New("invalid spend history date")
		}
		key := spendDayKey(row.Day, row.Provider, row.Model)
		if row.Provider == "" || row.Model == "" || seen[key] {
			return errors.New("invalid or duplicate spend history billing identity")
		}
		seen[key] = true
		for _, counter := range []int{row.Calls, row.Failures, row.InputTokens, row.OutputTokens, row.UnknownCostCalls, row.PrimaryCalls, row.ChallengerCalls, row.TestCalls} {
			if counter < 0 {
				return errors.New("negative spend history counter")
			}
		}
		if row.Failures > row.Calls || row.UnknownCostCalls > row.Calls || row.PrimaryCalls+row.ChallengerCalls+row.TestCalls != row.Calls {
			return errors.New("inconsistent spend history counters")
		}
		for _, cost := range []float64{row.CostUSD, row.ReportedCostUSD, row.EstimatedCostUSD} {
			if cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
				return errors.New("invalid spend history cost")
			}
		}
		if row.ReportedCostUSD+row.EstimatedCostUSD > row.CostUSD+1e-12 {
			return errors.New("inconsistent spend history cost sources")
		}
	}
	return nil
}

func utcSpendDay(now time.Time) time.Time {
	utc := now.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func spendDayKey(day, provider, model string) string { return day + "\x00" + provider + "\x00" + model }

func spendIdentity(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ToValidUTF8(value, ""), "\x00", ""))
	if value == "" {
		return "unknown"
	}
	return value
}
