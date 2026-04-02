package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"

	"github.com/rakibulbh/safe-london/internal/domain"
)

// ~0.01 degrees ≈ 1.1km at London's latitude
const searchRadiusDeg = 0.01

type Repository struct {
	db *sql.DB
}

func NewRepository(dsn string) (*Repository, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Repository{db: db}, nil
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS historical_crimes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			date TEXT NOT NULL,
			street TEXT NOT NULL,
			category TEXT NOT NULL,
			lat REAL NOT NULL,
			lng REAL NOT NULL,
			UNIQUE(date, street, category, lat, lng)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_crimes_location ON historical_crimes(lat, lng)`,
		`CREATE TABLE IF NOT EXISTS reports (
			id TEXT PRIMARY KEY,
			lat REAL NOT NULL,
			lng REAL NOT NULL,
			theft_type TEXT NOT NULL,
			is_threat INTEGER NOT NULL,
			threat_level INTEGER NOT NULL,
			description TEXT NOT NULL,
			trend_analysis TEXT NOT NULL,
			created_at DATETIME NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("exec %q: %w", s[:40], err)
		}
	}
	return nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) InsertCrimes(ctx context.Context, crimes []domain.HistoricalCrime) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO historical_crimes (date, street, category, lat, lng) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, c := range crimes {
		if _, err := stmt.ExecContext(ctx, c.Date, c.Street, c.Category, c.Lat, c.Lng); err != nil {
			return fmt.Errorf("insert crime: %w", err)
		}
	}
	return tx.Commit()
}

func (r *Repository) GetRecentCrimes(ctx context.Context, lat, lng float64) ([]domain.HistoricalCrime, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, date, street, category, lat, lng FROM historical_crimes
		 WHERE lat BETWEEN ? AND ? AND lng BETWEEN ? AND ?
		 ORDER BY date DESC LIMIT 50`,
		lat-searchRadiusDeg, lat+searchRadiusDeg,
		lng-searchRadiusDeg, lng+searchRadiusDeg,
	)
	if err != nil {
		return nil, fmt.Errorf("query crimes: %w", err)
	}
	defer rows.Close()

	var crimes []domain.HistoricalCrime
	for rows.Next() {
		var c domain.HistoricalCrime
		if err := rows.Scan(&c.ID, &c.Date, &c.Street, &c.Category, &c.Lat, &c.Lng); err != nil {
			return nil, fmt.Errorf("scan crime: %w", err)
		}
		crimes = append(crimes, c)
	}
	return crimes, rows.Err()
}

// Save persists an enriched alert (implements domain.ReportRepository).
func (r *Repository) Save(ctx context.Context, alert domain.EnrichedAlert) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO reports (id, lat, lng, theft_type, is_threat, threat_level, description, trend_analysis, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		alert.Report.ID,
		alert.Report.Location.Lat,
		alert.Report.Location.Lng,
		alert.Report.TheftType,
		alert.Analysis.IsThreat,
		alert.Analysis.ThreatLevel,
		alert.Analysis.Description,
		alert.Analysis.TrendAnalysis,
		alert.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save report: %w", err)
	}
	return nil
}
