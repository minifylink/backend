package repository

import (
	"backend/internal/config"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	saveLinkQuery = `INSERT INTO links (original_link, short_id, created_at) 
               VALUES ($1, $2, NOW())`
	getLinkQuery       = `SELECT id, original_link FROM links WHERE short_id = $1`
	saveAnalyticsQuery = `INSERT INTO analytics (link_id, timestamp, country, device, browser) 
                    VALUES ($1, NOW(), $2, $3, $4)`
)

type Storage struct {
	db  *sql.DB
	log *slog.Logger
}

type StatisticResponse struct {
	Clicks    int               `json:"clicks"`
	Devices   map[string]string `json:"devices"`
	Countries []string          `json:"countries"`
}

func New(cfg *config.Config, log *slog.Logger) (*Storage, error) {
	const op = "storage.postgres.New"
	logger := log.With(slog.String("op", op))
	logger.Info("connecting to db")
	db, err := sql.Open("pgx", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.PostgresConfig.Host,
		cfg.PostgresConfig.Port,
		cfg.PostgresConfig.Username,
		cfg.PostgresConfig.Password,
		cfg.PostgresConfig.DBName,
		cfg.PostgresConfig.SSLMode))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", "failed to open database connection", err)
	}

	stmt, err := db.Prepare("CREATE TABLE IF NOT EXISTS links (" +
		"id SERIAL PRIMARY KEY, " +
		"original_link TEXT NOT NULL, " +
		"short_id VARCHAR(20) NOT NULL UNIQUE, " +
		"created_at TIMESTAMP NOT NULL DEFAULT now())")

	if err != nil {
		return nil, fmt.Errorf("%s: %w", "failed to prepare create links table", err)
	}

	_, err = stmt.Exec()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", "failed to create links table", err)
	}

	stmt, err = db.Prepare("CREATE TABLE IF NOT EXISTS users (" +
		"id SERIAL PRIMARY KEY, " +
		"telegram_id BIGINT NOT NULL UNIQUE, " +
		"created_at TIMESTAMP NOT NULL DEFAULT now())")

	if err != nil {
		return nil, fmt.Errorf("%s: %w", "failed to prepare create users table", err)
	}

	_, err = stmt.Exec()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", "failed to create users table", err)
	}

	stmt, err = db.Prepare("CREATE TABLE IF NOT EXISTS analytics (" +
		"id SERIAL PRIMARY KEY, " +
		"link_id INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE, " +
		"timestamp TIMESTAMP NOT NULL DEFAULT now(), " +
		"country VARCHAR(50), " +
		"device VARCHAR(50), " +
		"browser VARCHAR(50))")

	if err != nil {
		return nil, fmt.Errorf("%s: %w", "failed to prepare create analytics table", err)
	}

	_, err = stmt.Exec()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", "failed to create analytics table", err)
	}

	logger.Info("successfully connected to db")

	return &Storage{db: db, log: log}, nil
}

func (s *Storage) SaveLink(originalLink string, shortID string) error {
	const op = "repository.SaveLink"
	logger := s.log.With(slog.String("op", op))

	_, err := s.db.Exec(saveLinkQuery, originalLink, shortID)
	if err != nil {
		if err.Error() == "pq: duplicate key value violates unique constraint \"links_short_id_key\"" {
			logger.Error("short id already exists", slog.String("error", err.Error()))
			return fmt.Errorf("%s: short id already exists", op)
		}

		logger.Error("failed to save link", slog.String("error", err.Error()))
		return fmt.Errorf("%s: %w", op, err)
	}

	logger.Info("Link successfully saved")
	return nil
}

func (s *Storage) GetLink(shortID string, country, device, browser string) (string, error) {
	const op = "repository.GetLink"
	logger := s.log.With(slog.String("op", op))

	tx, err := s.db.Begin()
	if err != nil {
		logger.Error("failed to start transaction", slog.String("error", err.Error()))
		return "", fmt.Errorf("%s: %w", op, err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var linkID int64
	var originalLink string

	err = tx.QueryRow(getLinkQuery, shortID).Scan(&linkID, &originalLink)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Error("link not found", slog.String("short_id", shortID))
			return "", fmt.Errorf("%s: link not found", op)
		}

		logger.Error("failed to get link", slog.String("error", err.Error()))
		return "", fmt.Errorf("%s: %w", op, err)
	}

	_, err = tx.Exec(saveAnalyticsQuery, linkID, country, device, browser)
	if err != nil {
		logger.Error("failed to save analytics", slog.String("error", err.Error()))
		return "", fmt.Errorf("%s: %w", op, err)
	}

	err = tx.Commit()
	if err != nil {
		logger.Error("failed to commit transaction", slog.String("error", err.Error()))
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return originalLink, nil
}

func (s *Storage) GetStatistic(shortID string) (*StatisticResponse, error) {
	const op = "repository.GetStatistic"
	logger := s.log.With(slog.String("op", op))

	var linkID int64
	err := s.db.QueryRow("SELECT id FROM links WHERE short_id = $1", shortID).Scan(&linkID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Error("link not found", slog.String("short_id", shortID))
			return nil, fmt.Errorf("%s: link not found", op)
		}
		logger.Error("failed to get link ID", slog.String("error", err.Error()))
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	var totalClicks int
	err = s.db.QueryRow("SELECT COUNT(*) FROM analytics WHERE link_id = $1", linkID).Scan(&totalClicks)
	if err != nil {
		logger.Error("failed to count clicks", slog.String("error", err.Error()))
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if totalClicks == 0 {
		return &StatisticResponse{
			Clicks:    0,
			Devices:   make(map[string]string),
			Countries: []string{},
		}, nil
	}

	deviceRows, err := s.db.Query(
		"SELECT device, COUNT(*) AS count FROM analytics WHERE link_id = $1 GROUP BY device ORDER BY count DESC",
		linkID,
	)
	if err != nil {
		logger.Error("failed to get device statistics", slog.String("error", err.Error()))
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer deviceRows.Close()

	devices := make(map[string]string)
	for deviceRows.Next() {
		var device string
		var count int
		if err := deviceRows.Scan(&device, &count); err != nil {
			logger.Error("failed to scan device row", slog.String("error", err.Error()))
			continue
		}
		percentage := float64(count) / float64(totalClicks) * 100
		devices[device] = fmt.Sprintf("%.0f%%", percentage)
	}

	countryRows, err := s.db.Query(
		"SELECT country FROM analytics WHERE link_id = $1 AND country != '' GROUP BY country ORDER BY COUNT(*) DESC",
		linkID,
	)
	if err != nil {
		logger.Error("failed to get country statistics", slog.String("error", err.Error()))
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer countryRows.Close()

	var countries []string
	for countryRows.Next() {
		var country string
		if err := countryRows.Scan(&country); err != nil {
			logger.Error("failed to scan country row", slog.String("error", err.Error()))
			continue
		}
		countries = append(countries, country)
	}

	return &StatisticResponse{
		Clicks:    totalClicks,
		Devices:   devices,
		Countries: countries,
	}, nil
}
