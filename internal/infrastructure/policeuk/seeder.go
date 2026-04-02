package policeuk

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/rakibulbh/safe-london/internal/domain"
	sqliterepo "github.com/rakibulbh/safe-london/internal/infrastructure/sqlite"
)

// London grid bounds at ~5.5km resolution
const gridStep = 0.05

var londonGrid = generateGrid(51.30, 51.65, -0.45, 0.28, gridStep)

type policeAPICrime struct {
	Category string `json:"category"`
	Location struct {
		Street struct {
			Name string `json:"name"`
		} `json:"street"`
		Latitude  string `json:"latitude"`
		Longitude string `json:"longitude"`
	} `json:"location"`
	Month string `json:"month"`
}

type gridPoint struct {
	Lat float64
	Lng float64
}

func generateGrid(minLat, maxLat, minLng, maxLng, step float64) []gridPoint {
	var points []gridPoint
	for lat := minLat; lat <= maxLat; lat += step {
		for lng := minLng; lng <= maxLng; lng += step {
			points = append(points, gridPoint{lat, lng})
		}
	}
	return points
}

// SeedHistoricalData fetches crime data for a coarse London grid and stores it locally.
func SeedHistoricalData(ctx context.Context, repo *sqliterepo.Repository, client *http.Client) {
	// Use 2 months prior to ensure data availability
	targetDate := time.Now().AddDate(0, -2, 0).Format("2006-01")
	slog.Info("starting historical data seed", "date", targetDate, "grid_points", len(londonGrid))

	for i, pt := range londonGrid {
		select {
		case <-ctx.Done():
			slog.Info("seed cancelled", "completed", i)
			return
		default:
		}

		crimes, err := fetchCrimes(ctx, client, pt.Lat, pt.Lng, targetDate)
		if err != nil {
			slog.Warn("failed to fetch crimes for grid point", "lat", pt.Lat, "lng", pt.Lng, "err", err)
			continue
		}

		if len(crimes) > 0 {
			if err := repo.InsertCrimes(ctx, crimes); err != nil {
				slog.Warn("failed to insert crimes", "err", err)
			}
		}

		// Respect rate limits
		time.Sleep(200 * time.Millisecond)

		if (i+1)%20 == 0 {
			slog.Info("seed progress", "completed", i+1, "total", len(londonGrid))
		}
	}
	slog.Info("historical data seed complete")
}

func fetchCrimes(ctx context.Context, client *http.Client, lat, lng float64, date string) ([]domain.HistoricalCrime, error) {
	url := fmt.Sprintf("https://data.police.uk/api/crimes-street/all-crime?lat=%f&lng=%f&date=%s", lat, lng, date)
	return fetchCrimesFromURL(ctx, client, url)
}

func fetchCrimesFromURL(ctx context.Context, client *http.Client, url string) ([]domain.HistoricalCrime, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("police api returned %d", resp.StatusCode)
	}

	var raw []policeAPICrime
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	crimes := make([]domain.HistoricalCrime, 0, len(raw))
	for _, r := range raw {
		var cLat, cLng float64
		fmt.Sscanf(r.Location.Latitude, "%f", &cLat)
		fmt.Sscanf(r.Location.Longitude, "%f", &cLng)

		crimes = append(crimes, domain.HistoricalCrime{
			Date:     r.Month,
			Street:   r.Location.Street.Name,
			Category: r.Category,
			Lat:      cLat,
			Lng:      cLng,
		})
	}
	return crimes, nil
}
