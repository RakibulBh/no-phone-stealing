package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/rakibulbh/safe-london/internal/domain"
)

func setupTestDB(t *testing.T) *Repository {
	t.Helper()
	repo, err := NewRepository(":memory:")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

func TestNewRepository_CreatesTable(t *testing.T) {
	repo := setupTestDB(t)

	var count int
	err := repo.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='historical_crimes'").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("expected historical_crimes table to exist, got count=%d", count)
	}
}

func TestInsertAndGetRecentCrimes(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	crimes := []domain.HistoricalCrime{
		{Date: "2024-01", Street: "Oxford Street", Category: "robbery", Lat: 51.5155, Lng: -0.1415},
		{Date: "2024-01", Street: "Baker Street", Category: "theft-from-the-person", Lat: 51.5237, Lng: -0.1585},
		{Date: "2024-02", Street: "Far Away Road", Category: "burglary", Lat: 51.60, Lng: 0.10},
	}

	if err := repo.InsertCrimes(ctx, crimes); err != nil {
		t.Fatalf("InsertCrimes failed: %v", err)
	}

	// Query near Oxford/Baker Street — should get 2 results within ~1km radius
	got, err := repo.GetRecentCrimes(ctx, 51.52, -0.15)
	if err != nil {
		t.Fatalf("GetRecentCrimes failed: %v", err)
	}

	if len(got) < 2 {
		t.Errorf("expected at least 2 nearby crimes, got %d", len(got))
	}

	// Verify no far-away crimes snuck in (Far Away Road is ~20km away)
	for _, c := range got {
		if c.Street == "Far Away Road" {
			t.Error("should not return crimes far outside the search radius")
		}
	}
}

func TestGetRecentCrimes_EmptyDB(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	got, err := repo.GetRecentCrimes(ctx, 51.5074, -0.1278)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 crimes from empty db, got %d", len(got))
	}
}

func TestSaveAlert_And_ReportsTable(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	alert := domain.EnrichedAlert{
		Report: domain.Report{
			ID:        "test-123",
			Location:  domain.Location{Lat: 51.5074, Lng: -0.1278},
			TheftType: "phone_snatch",
		},
		Analysis: domain.LLMAnalysisResult{
			IsThreat:      true,
			ThreatLevel:   4,
			Description:   "Active theft spotted",
			TrendAnalysis: "Suspects flee north",
		},
		CreatedAt: time.Now(),
	}

	if err := repo.Save(ctx, alert); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	var count int
	err := repo.db.QueryRow("SELECT count(*) FROM reports WHERE id = ?", "test-123").Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 saved report, got %d", count)
	}
}

func TestInsertCrimes_Idempotent(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	crime := domain.HistoricalCrime{Date: "2024-01", Street: "Oxford Street", Category: "robbery", Lat: 51.5155, Lng: -0.1415}

	// Insert same crime twice — should not fail or duplicate
	if err := repo.InsertCrimes(ctx, []domain.HistoricalCrime{crime}); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}
	if err := repo.InsertCrimes(ctx, []domain.HistoricalCrime{crime}); err != nil {
		t.Fatalf("second insert should not fail: %v", err)
	}
}
