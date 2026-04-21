// Package statestore persists enrolled phones and server config to SQLite.
package statestore

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS phones (
  nickname        TEXT PRIMARY KEY,
  tailscale_host  TEXT,
  last_ws_seen    INTEGER,
  paired          INTEGER NOT NULL DEFAULT 0,
  adb_fingerprint TEXT
);
CREATE TABLE IF NOT EXISTS server_config (
  id              INTEGER PRIMARY KEY CHECK (id = 1),
  psk             BLOB NOT NULL,
  ws_port         INTEGER NOT NULL,
  tailscale_host  TEXT NOT NULL
);
`

type Phone struct {
	Nickname       string
	TailscaleHost  string
	LastWSSeen     time.Time
	Paired         bool
	ADBFingerprint string
}

type ServerConfig struct {
	PSK           []byte
	WSPort        int
	TailscaleHost string
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) SetServerConfig(c *ServerConfig) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO server_config (id, psk, ws_port, tailscale_host) VALUES (1, ?, ?, ?)`,
		c.PSK, c.WSPort, c.TailscaleHost)
	return err
}

func (s *Store) ServerConfig() (*ServerConfig, error) {
	row := s.db.QueryRow(`SELECT psk, ws_port, tailscale_host FROM server_config WHERE id = 1`)
	var c ServerConfig
	if err := row.Scan(&c.PSK, &c.WSPort, &c.TailscaleHost); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (s *Store) UpsertPhone(p Phone) error {
	paired := 0
	if p.Paired {
		paired = 1
	}
	_, err := s.db.Exec(`
INSERT INTO phones (nickname, tailscale_host, paired, adb_fingerprint)
VALUES (?, ?, ?, ?)
ON CONFLICT(nickname) DO UPDATE SET
  tailscale_host  = excluded.tailscale_host,
  paired          = excluded.paired,
  adb_fingerprint = COALESCE(excluded.adb_fingerprint, phones.adb_fingerprint)`,
		p.Nickname, nullString(p.TailscaleHost), paired, nullString(p.ADBFingerprint))
	return err
}

func (s *Store) RecordPhoneSeen(nickname string) error {
	_, err := s.db.Exec(`UPDATE phones SET last_ws_seen = ? WHERE nickname = ?`, time.Now().UnixMilli(), nickname)
	return err
}

func (s *Store) MarkPaired(nickname, fingerprint string) error {
	_, err := s.db.Exec(`UPDATE phones SET paired = 1, adb_fingerprint = ? WHERE nickname = ?`, fingerprint, nickname)
	return err
}

func (s *Store) GetPhone(nickname string) (*Phone, error) {
	row := s.db.QueryRow(`SELECT nickname, COALESCE(tailscale_host,''), COALESCE(last_ws_seen,0), paired, COALESCE(adb_fingerprint,'') FROM phones WHERE nickname = ?`, nickname)
	var p Phone
	var lastMS int64
	var paired int
	if err := row.Scan(&p.Nickname, &p.TailscaleHost, &lastMS, &paired, &p.ADBFingerprint); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if lastMS > 0 {
		p.LastWSSeen = time.UnixMilli(lastMS)
	}
	p.Paired = paired == 1
	return &p, nil
}

func (s *Store) ListPhones() ([]Phone, error) {
	rows, err := s.db.Query(`SELECT nickname, COALESCE(tailscale_host,''), COALESCE(last_ws_seen,0), paired, COALESCE(adb_fingerprint,'') FROM phones ORDER BY nickname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Phone
	for rows.Next() {
		var p Phone
		var lastMS int64
		var paired int
		if err := rows.Scan(&p.Nickname, &p.TailscaleHost, &lastMS, &paired, &p.ADBFingerprint); err != nil {
			return nil, err
		}
		if lastMS > 0 {
			p.LastWSSeen = time.UnixMilli(lastMS)
		}
		p.Paired = paired == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
