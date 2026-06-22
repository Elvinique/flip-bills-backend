package providers

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ─── Circuit Breaker State ────────────────────────────────────────────────────

type breakerState struct {
	failures    int
	lastFailure time.Time
	open        bool
}

// ─── ProviderRouter ───────────────────────────────────────────────────────────

// ProviderRouter selects the best available PaymentProvider for a given
// feature using priority lists. It integrates an inline circuit breaker
// to automatically skip unhealthy providers and route to the next one.
//
// Priority lists are configured at startup:
//
//	"card"    -> [paystack, flutterwave, monnify]
//	"bills"   -> [opay, flutterwave, monnify]
//	"virtual" -> [monnify, paystack]
//	"transfer"-> [paystack, flutterwave]
type ProviderRouter struct {
	mu               sync.Mutex
	providers        map[string]PaymentProvider
	priorityLists    map[string][]string
	breakerStates    map[string]*breakerState
	failureThreshold int
	resetTimeout     time.Duration
	log              *zap.Logger
}

func NewProviderRouter(
	provs []PaymentProvider,
	failureThreshold int,
	resetTimeout time.Duration,
	log *zap.Logger,
) *ProviderRouter {
	pm := make(map[string]PaymentProvider, len(provs))
	bs := make(map[string]*breakerState, len(provs))
	for _, p := range provs {
		pm[p.Name()] = p
		bs[p.Name()] = &breakerState{}
	}

	return &ProviderRouter{
		providers: pm,
		priorityLists: map[string][]string{
			"card":     {"paystack", "flutterwave", "monnify"},
			"bills":    {"opay", "flutterwave", "monnify"},
			"virtual":  {"monnify", "paystack"},
			"transfer": {"paystack", "flutterwave"},
			"default":  {"flutterwave", "monnify"},
		},
		breakerStates:    bs,
		failureThreshold: failureThreshold,
		resetTimeout:     resetTimeout,
		log:              log,
	}
}

// Select returns the first healthy provider for the requested feature.
// Returns nil only if ALL providers in the priority list are open-circuited.
func (r *ProviderRouter) Select(ctx context.Context, feature string) PaymentProvider {
	list, ok := r.priorityLists[feature]
	if !ok {
		list = r.priorityLists["default"]
	}

	for _, name := range list {
		p, exists := r.providers[name]
		if !exists {
			continue
		}
		if r.isOpen(name) {
			r.log.Warn("circuit open, skipping provider", zap.String("provider", name))
			continue
		}
		if p.HealthCheck(ctx) {
			return p
		}
		r.recordFailure(name)
	}

	r.log.Error("all providers unavailable for feature", zap.String("feature", feature))
	return nil
}

// RecordSuccess resets the failure counter for a provider.
func (r *ProviderRouter) RecordSuccess(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.breakerStates[name]; ok {
		s.failures = 0
		s.open = false
	}
}

// RecordFailure increments the failure counter. Opens the circuit when
// the threshold is exceeded.
func (r *ProviderRouter) RecordFailure(name string) {
	r.recordFailure(name)
}

func (r *ProviderRouter) recordFailure(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.breakerStates[name]
	if !ok {
		return
	}
	s.failures++
	s.lastFailure = time.Now()
	if s.failures >= r.failureThreshold {
		s.open = true
		r.log.Warn("circuit breaker OPEN",
			zap.String("provider", name),
			zap.Int("failures", s.failures))
	}
}

func (r *ProviderRouter) isOpen(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.breakerStates[name]
	if !ok {
		return false
	}
	if s.open && time.Since(s.lastFailure) > r.resetTimeout {
		// Half-open: allow one probe
		s.open = false
		s.failures = 0
		r.log.Info("circuit breaker half-open, probing", zap.String("provider", name))
	}
	return s.open
}
