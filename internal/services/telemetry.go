package services

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"capcom/internal/domain"
)

var (
	ErrInvalidUsageQuery       = errors.New("invalid usage query")
	ErrInvalidUsageObservation = errors.New("invalid usage observation")
)

type TelemetryRepository interface {
	UpsertObservation(ctx context.Context, observation domain.UsageObservation) (bool, error)
	CreateIngestionRun(ctx context.Context, run domain.TelemetryIngestionRun) (domain.TelemetryIngestionRun, error)
	FinishIngestionRun(ctx context.Context, run domain.TelemetryIngestionRun, runErr error) (domain.TelemetryIngestionRun, error)
	LatestIngestionRun(ctx context.Context, runtimeID string) (domain.TelemetryIngestionRun, error)
	Summary(ctx context.Context, query domain.UsageQuery) (domain.UsageSummary, error)
}

type ModelMetadata struct {
	Provider              string
	Model                 string
	ContextWindowTokens   int64
	InputCostPer1MTokens  *domain.Decimal
	OutputCostPer1MTokens *domain.Decimal
	Version               string
}

type ModelMetadataResolver interface {
	Resolve(ctx context.Context, provider, model string) (ModelMetadata, bool, error)
}

type TelemetryService struct {
	repository TelemetryRepository
	models     ModelMetadataResolver
	now        func() time.Time
}

func NewTelemetryService(repository TelemetryRepository) TelemetryService {
	return TelemetryService{repository: repository, now: func() time.Time { return time.Now().UTC() }}
}

func (s TelemetryService) WithModelMetadataResolver(resolver ModelMetadataResolver) TelemetryService {
	s.models = resolver
	return s
}

func (s TelemetryService) Ingest(ctx context.Context, observations []domain.UsageObservation) (accepted, rejected, deduplicated int64, err error) {
	for _, observation := range observations {
		if validationErr := s.normalize(ctx, &observation); validationErr != nil {
			rejected++
			if err == nil {
				err = validationErr
			}
			continue
		}
		inserted, persistErr := s.repository.UpsertObservation(ctx, observation)
		if persistErr != nil {
			if errors.Is(persistErr, domain.ErrUnknownRuntimeConnection) {
				rejected++
				if err == nil {
					err = persistErr
				}
				continue
			}
			return accepted, rejected, deduplicated, persistErr
		}
		if inserted {
			accepted++
		} else {
			deduplicated++
		}
	}
	return accepted, rejected, deduplicated, err
}

func (s TelemetryService) Summary(ctx context.Context, query domain.UsageQuery) (domain.UsageSummary, error) {
	if query.To.IsZero() {
		query.To = s.now()
	}
	if query.From.IsZero() {
		query.From = query.To.Add(-24 * time.Hour)
	}
	if !query.From.Before(query.To) || query.To.Sub(query.From) > 90*24*time.Hour {
		return domain.UsageSummary{}, fmt.Errorf("%w: from must precede to and range must not exceed 90 days", ErrInvalidUsageQuery)
	}
	return s.repository.Summary(ctx, query)
}

func (s TelemetryService) Health(ctx context.Context, runtimeID string) (domain.TelemetryIngestionRun, error) {
	if strings.TrimSpace(runtimeID) == "" {
		return domain.TelemetryIngestionRun{}, fmt.Errorf("%w: runtime id is required", ErrInvalidUsageQuery)
	}
	return s.repository.LatestIngestionRun(ctx, runtimeID)
}

func (s TelemetryService) normalize(ctx context.Context, observation *domain.UsageObservation) error {
	if observation.Source == "" || strings.TrimSpace(observation.SourceObservationID) == "" ||
		strings.TrimSpace(observation.RuntimeConnectionID) == "" {
		return fmt.Errorf("%w: source, source observation id, and runtime connection id are required", ErrInvalidUsageObservation)
	}
	counts := []int64{
		observation.RequestCount, observation.InputTokens, observation.OutputTokens,
		observation.CachedInputTokens, observation.CacheCreationTokens,
		observation.ReasoningTokens, observation.TotalTokens, observation.ErrorCount,
	}
	for _, count := range counts {
		if count < 0 {
			return fmt.Errorf("%w: token and request counts cannot be negative", ErrInvalidUsageObservation)
		}
	}
	if observation.TotalTokens == 0 {
		observation.TotalTokens = observation.InputTokens + observation.OutputTokens
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = s.now()
	}
	if observation.Attributes == nil {
		observation.Attributes = map[string]any{}
	}
	delete(observation.Attributes, "prompt")
	delete(observation.Attributes, "completion")
	delete(observation.Attributes, "input")
	delete(observation.Attributes, "output")

	if s.models != nil && observation.Model != "" {
		metadata, found, err := s.models.Resolve(ctx, observation.Provider, observation.Model)
		if err != nil {
			return fmt.Errorf("resolve model metadata: %w", err)
		}
		if found {
			observation.ModelMetadataVersion = metadata.Version
			if observation.ContextWindowTokens == nil && metadata.ContextWindowTokens > 0 {
				observation.ContextWindowTokens = &metadata.ContextWindowTokens
			}
			if observation.InputCostUSD == nil && metadata.InputCostPer1MTokens != nil {
				observation.InputCostUSD, err = estimatedTokenCost(observation.InputTokens, *metadata.InputCostPer1MTokens)
				if err != nil {
					return fmt.Errorf("estimate input cost: %w", err)
				}
			}
			if observation.OutputCostUSD == nil && metadata.OutputCostPer1MTokens != nil {
				observation.OutputCostUSD, err = estimatedTokenCost(observation.OutputTokens, *metadata.OutputCostPer1MTokens)
				if err != nil {
					return fmt.Errorf("estimate output cost: %w", err)
				}
			}
			if observation.TotalCostUSD == nil {
				observation.TotalCostUSD, err = sumDecimalPointers(observation.InputCostUSD, observation.OutputCostUSD)
				if err != nil {
					return fmt.Errorf("estimate total cost: %w", err)
				}
			}
			if observation.InputCostUSD != nil || observation.OutputCostUSD != nil {
				observation.Attributes["cost_quality"] = "estimated"
				observation.Attributes["model_metadata_version"] = metadata.Version
			}
			if observation.CachedInputTokens > 0 && metadata.InputCostPer1MTokens != nil {
				cacheSavings, savingsErr := estimatedTokenCost(observation.CachedInputTokens, *metadata.InputCostPer1MTokens)
				if savingsErr != nil {
					return fmt.Errorf("estimate cache savings: %w", savingsErr)
				}
				observation.Attributes["estimated_cache_savings_usd"] = string(*cacheSavings)
			}
		}
	}
	if observation.ContextWindowTokens != nil && *observation.ContextWindowTokens > 0 {
		utilization := float64(observation.InputTokens) / float64(*observation.ContextWindowTokens)
		observation.ContextUtilization = &utilization
		observation.Attributes["context_utilization_quality"] = "estimated"
		switch {
		case utilization >= 0.9:
			observation.Attributes["context_pressure"] = "critical"
		case utilization >= 0.8:
			observation.Attributes["context_pressure"] = "high"
		}
	}
	return nil
}

func estimatedTokenCost(tokens int64, pricePerMillion domain.Decimal) (*domain.Decimal, error) {
	price, ok := new(big.Rat).SetString(string(pricePerMillion))
	if !ok {
		return nil, fmt.Errorf("invalid catalog decimal %q", pricePerMillion)
	}
	cost := new(big.Rat).Mul(price, new(big.Rat).SetInt64(tokens))
	cost.Quo(cost, new(big.Rat).SetInt64(1_000_000))
	value := decimalFromRat(cost)
	return &value, nil
}

func sumDecimalPointers(values ...*domain.Decimal) (*domain.Decimal, error) {
	total := new(big.Rat)
	found := false
	for _, value := range values {
		if value == nil {
			continue
		}
		parsed, ok := new(big.Rat).SetString(string(*value))
		if !ok {
			return nil, fmt.Errorf("invalid decimal %q", *value)
		}
		total.Add(total, parsed)
		found = true
	}
	if !found {
		return nil, nil
	}
	value := decimalFromRat(total)
	return &value, nil
}

func decimalFromRat(value *big.Rat) domain.Decimal {
	text := strings.TrimRight(strings.TrimRight(value.FloatString(10), "0"), ".")
	if text == "" {
		text = "0"
	}
	return domain.Decimal(text)
}
