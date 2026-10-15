// Package store keeps the service's data in MySQL: users and sessions,
// finished game reviews, and the rest as the stages of WEB-PLAN.md arrive.
//
// The schema is versioned by the numbered files in migrations/, applied in
// order at start-up and recorded in the schema_migrations table. The
// runner is deliberately tiny (embedded files, one lock, one table): the
// schema is ours alone and changes by adding files, never by editing them.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("store: not found")

// Store is a handle on the database.
type Store struct {
	db *sql.DB
}

// Open connects to MySQL (dsn in the go-sql-driver format, e.g.
// "user:pass@tcp(db:3306)/dyanis") and brings the schema up to date. The
// database may still be starting, so the first connection is retried for
// up to wait.
func Open(ctx context.Context, dsn string, wait time.Duration) (*Store, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: bad DSN: %w", err)
	}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.MultiStatements = true // a migration file may hold several statements
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["charset"] = "utf8mb4"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	deadline := time.Now().Add(wait)
	for {
		err = db.PingContext(ctx)
		if err == nil {
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			db.Close()
			return nil, fmt.Errorf("store: database not reachable: %w", err)
		}
		time.Sleep(time.Second)
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the connections.
func (s *Store) Close() error { return s.db.Close() }

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	seen := map[int]string{}
	for _, e := range entries {
		name := e.Name()
		num, _, ok := strings.Cut(name, "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil || !strings.HasSuffix(name, ".sql") {
			return nil, fmt.Errorf("store: migration file %q must be named <number>_<name>.sql", name)
		}
		if prev, dup := seen[v]; dup {
			return nil, fmt.Errorf("store: migrations %q and %q share version %d", prev, name, v)
		}
		seen[v] = name
		data, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: name, sql: string(data)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// migrate applies the migrations not yet recorded. A named lock keeps two
// servers starting together from applying the same file twice. MySQL DDL
// commits implicitly, so a file that fails half-way is not rolled back:
// keep each file small and make fixes in a new one.
func (s *Store) migrate(ctx context.Context) error {
	migs, err := loadMigrations()
	if err != nil {
		return err
	}
	// The lock belongs to one connection, so use one for the whole run.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	var got int
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK('dyanis_migrate', 60)").Scan(&got); err != nil || got != 1 {
		return fmt.Errorf("store: could not take the migration lock (%v)", err)
	}
	defer conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK('dyanis_migrate')")

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INT         NOT NULL PRIMARY KEY,
		name       VARCHAR(100) NOT NULL,
		applied_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return err
	}
	done := map[int]bool{}
	rows, err := conn.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		done[v] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, m := range migs {
		if done[m.version] {
			continue
		}
		if _, err := conn.ExecContext(ctx, m.sql); err != nil {
			return fmt.Errorf("store: migration %s: %w", m.name, err)
		}
		if _, err := conn.ExecContext(ctx, "INSERT INTO schema_migrations (version, name) VALUES (?, ?)", m.version, m.name); err != nil {
			return err
		}
	}
	return nil
}
