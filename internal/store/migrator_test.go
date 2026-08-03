package store

import (
	"strings"
	"testing"
)

func TestLoadMigrations(t *testing.T) {
	migrations, err := LoadMigrations("../../migrations")
	if err != nil {
		t.Fatalf("LoadMigrations returned error: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("LoadMigrations returned no migrations")
	}
	if migrations[0].Version != "001_initial_schema" {
		t.Fatalf("first migration = %q, want 001_initial_schema", migrations[0].Version)
	}
	if !strings.Contains(migrations[0].SQL, "CREATE TABLE IF NOT EXISTS runtime_connections") {
		t.Fatal("initial migration does not create runtime_connections")
	}
	if len(migrations) < 2 || !strings.Contains(migrations[1].SQL, "CREATE TABLE IF NOT EXISTS secrets") {
		t.Fatal("secret storage migration is missing")
	}
	if len(migrations) < 3 || !strings.Contains(migrations[2].SQL, "CREATE TABLE IF NOT EXISTS runtime_sync_runs") {
		t.Fatal("runtime sync migration is missing")
	}
	if len(migrations) < 5 || !strings.Contains(migrations[4].SQL, "ADD COLUMN IF NOT EXISTS display_name") {
		t.Fatal("runtime instance identity migration is missing")
	}
	telemetryFound := false
	for _, migration := range migrations {
		if strings.Contains(migration.SQL, "CREATE TABLE IF NOT EXISTS usage_observations") &&
			strings.Contains(migration.SQL, "CREATE TABLE IF NOT EXISTS telemetry_ingestion_runs") {
			telemetryFound = true
			break
		}
	}
	if !telemetryFound {
		t.Fatal("telemetry migration is missing")
	}
}
