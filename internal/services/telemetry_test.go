package services

import (
	"context"
	"testing"
	"time"

	"capcom/internal/domain"
)

type telemetryRepositoryStub struct {
	observations []domain.UsageObservation
}

func (r *telemetryRepositoryStub) UpsertObservation(_ context.Context, observation domain.UsageObservation) (bool, error) {
	r.observations = append(r.observations, observation)
	return true, nil
}
func (*telemetryRepositoryStub) CreateIngestionRun(context.Context, domain.TelemetryIngestionRun) (domain.TelemetryIngestionRun, error) {
	return domain.TelemetryIngestionRun{}, nil
}
func (*telemetryRepositoryStub) FinishIngestionRun(context.Context, domain.TelemetryIngestionRun, error) (domain.TelemetryIngestionRun, error) {
	return domain.TelemetryIngestionRun{}, nil
}
func (*telemetryRepositoryStub) LatestIngestionRun(context.Context, string) (domain.TelemetryIngestionRun, error) {
	return domain.TelemetryIngestionRun{}, nil
}
func (*telemetryRepositoryStub) Summary(_ context.Context, query domain.UsageQuery) (domain.UsageSummary, error) {
	return domain.UsageSummary{From: query.From, To: query.To}, nil
}

func TestTelemetryServiceNormalizesAndRedactsObservation(t *testing.T) {
	repository := &telemetryRepositoryStub{}
	inputPrice := domain.Decimal("2")
	outputPrice := domain.Decimal("4")
	service := NewTelemetryService(repository).WithModelMetadataResolver(NewModelCatalog([]ModelMetadata{{
		Model: "test-model", ContextWindowTokens: 1000, Version: "catalog-v1",
		InputCostPer1MTokens: &inputPrice, OutputCostPer1MTokens: &outputPrice,
	}}))
	accepted, rejected, _, err := service.Ingest(context.Background(), []domain.UsageObservation{{
		Source: domain.UsageSourceOTEL, SourceObservationID: "span-1",
		RuntimeConnectionID: "runtime-1", Model: "test-model",
		InputTokens: 250, OutputTokens: 50,
		Attributes: map[string]any{"prompt": "secret", "safe": "value"},
	}})
	if err != nil || accepted != 1 || rejected != 0 {
		t.Fatalf("Ingest() = accepted %d rejected %d err %v", accepted, rejected, err)
	}
	got := repository.observations[0]
	if got.TotalTokens != 300 {
		t.Fatalf("TotalTokens = %d, want 300", got.TotalTokens)
	}
	if got.ContextUtilization == nil || *got.ContextUtilization != 0.25 {
		t.Fatalf("ContextUtilization = %v, want 0.25", got.ContextUtilization)
	}
	if _, stored := got.Attributes["prompt"]; stored {
		t.Fatal("prompt content was retained")
	}
	if got.ModelMetadataVersion != "catalog-v1" {
		t.Fatalf("ModelMetadataVersion = %q", got.ModelMetadataVersion)
	}
	if got.TotalCostUSD == nil || string(*got.TotalCostUSD) != "0.0007" {
		t.Fatalf("TotalCostUSD = %v, want 0.0007", got.TotalCostUSD)
	}
}

func TestTelemetryServiceRejectsInvalidRange(t *testing.T) {
	service := NewTelemetryService(&telemetryRepositoryStub{})
	_, err := service.Summary(context.Background(), domain.UsageQuery{
		From: time.Now(), To: time.Now().Add(-time.Hour),
	})
	if err == nil {
		t.Fatal("Summary() error = nil, want invalid range")
	}
}
