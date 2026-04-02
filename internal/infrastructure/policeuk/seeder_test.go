package policeuk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rakibulbh/safe-london/internal/domain"
)

func TestFetchCrimes_ParsesResponse(t *testing.T) {
	apiResp := []policeAPICrime{
		{
			Category: "robbery",
			Month:    "2024-01",
			Location: struct {
				Street struct {
					Name string `json:"name"`
				} `json:"street"`
				Latitude  string `json:"latitude"`
				Longitude string `json:"longitude"`
			}{
				Street: struct {
					Name string `json:"name"`
				}{Name: "Oxford Street"},
				Latitude:  "51.5155",
				Longitude: "-0.1415",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(apiResp)
	}))
	defer server.Close()

	// Override fetchCrimes to use test server — we test the parsing logic directly
	crimes, err := fetchCrimesFromURL(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetchCrimesFromURL failed: %v", err)
	}

	if len(crimes) != 1 {
		t.Fatalf("expected 1 crime, got %d", len(crimes))
	}
	if crimes[0].Street != "Oxford Street" {
		t.Errorf("expected Oxford Street, got %s", crimes[0].Street)
	}
	if crimes[0].Category != "robbery" {
		t.Errorf("expected robbery, got %s", crimes[0].Category)
	}
}

func TestFetchCrimes_HandlesNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := fetchCrimesFromURL(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Error("expected error for non-200 response")
	}
}

func TestFetchCrimes_HandlesInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	_, err := fetchCrimesFromURL(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGenerateGrid_ProducesPoints(t *testing.T) {
	grid := generateGrid(51.3, 51.4, -0.2, -0.1, 0.05)
	if len(grid) == 0 {
		t.Error("expected non-empty grid")
	}

	for _, pt := range grid {
		if pt.Lat < 51.3 || pt.Lat > 51.4 {
			t.Errorf("lat %f out of bounds", pt.Lat)
		}
		if pt.Lng < -0.2 || pt.Lng > -0.1 {
			t.Errorf("lng %f out of bounds", pt.Lng)
		}
	}
}

func TestFormatCrimesIntegration(t *testing.T) {
	crimes := []domain.HistoricalCrime{
		{Date: "2024-01", Street: "Oxford Street", Category: "robbery"},
	}
	text := domain.FormatCrimesAsText(crimes)
	expected := "On 2024-01 at Oxford Street, robbery occurred."
	if text != expected {
		t.Errorf("expected %q, got %q", expected, text)
	}
}
