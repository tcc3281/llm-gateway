package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"llm-gateway/internal/config"

	_ "github.com/lib/pq"
)

type PostgresDB struct {
	*sql.DB
}

func NewPostgres(cfg *config.Config) (*PostgresDB, error) {
	dsn := cfg.GetDSN()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	pg := &PostgresDB{DB: db}
	if err := pg.runMigrations(); err != nil {
		log.Printf("Warning running migrations: %v", err)
	}

	return pg, nil
}

func (p *PostgresDB) runMigrations() error {
	dir := "migrations"
	if _, err := os.Stat(dir); err != nil {
		dir = "../../migrations" // Fallback when running from cmd/
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("Warning: cannot read migrations dir %q: %v", dir, err)
		return nil
	}

	// Apply in filename order so 000001 runs before 000002, etc.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	applied := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return fmt.Errorf("failed reading migration %s: %w", e.Name(), err)
		}
		if _, err := p.Exec(string(content)); err != nil {
			return fmt.Errorf("failed executing migration %s: %w", e.Name(), err)
		}
		applied++
		log.Printf("Applied migration %s", e.Name())
	}

	log.Printf("PostgreSQL schema migration completed (%d file(s)).", applied)
	return nil
}
