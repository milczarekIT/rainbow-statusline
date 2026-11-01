package integration

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	sqlite "modernc.org/sqlite"
)

const (
	busyTimeout = "500"
	openLimit   = time.Second
	cantOpen    = 14 // SQLITE_CANTOPEN
)

func dsn(path string, immutable bool) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // C:/dir/x.db -> /C:/dir/x.db
	}
	q := url.Values{}
	if immutable {
		q.Set("immutable", "1")
	} else {
		q.Set("mode", "ro")
	}
	q.Add("_pragma", "busy_timeout("+busyTimeout+")")
	return (&url.URL{Scheme: "file", Path: p, RawQuery: q.Encode()}).String()
}

// OpenReadOnly opens an existing database without ever writing to it. It reads
// through the normal locking protocol, so a live WAL database shows its latest
// commits and a lock held by a writer ends in a 500 ms timeout, never in stale
// data. Only when SQLite cannot open the file at all (a WAL-format file with no
// -shm/-wal beside it, which a read-only handle cannot create, as RTK's history
// is) does it retry with immutable=1, where nothing else can be missing.
func OpenReadOnly(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := openCheck(dsn(path, false))
	if err != nil && isCantOpen(err) {
		db, err = openCheck(dsn(path, true))
	}
	return db, err
}

func openCheck(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), openLimit)
	defer cancel()
	var n int
	if err := db.QueryRowContext(ctx, "select count(*) from sqlite_master").Scan(&n); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func isCantOpen(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code()&0xff == cantOpen
}

// pair runs a query that returns two integer columns; NULL counts as 0.
func pair(ctx context.Context, db *sql.DB, query string, args ...any) (a, b int64, err error) {
	var x, y sql.NullInt64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&x, &y); err != nil {
		return 0, 0, err
	}
	return x.Int64, y.Int64, nil
}
