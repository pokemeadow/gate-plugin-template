package pokeperms

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite" // Pure Go SQLite driver (No CGO required)
)

// Storage handles all database interactions for Gate Proxy
type Storage struct {
	db          *sql.DB
	storageType string
}

// NewStorage initializes the database connection and creates necessary tables
func NewStorage(dir string, cfg *Config) (*Storage, error) {
	var db *sql.DB
	var err error

	if cfg.StorageType == "mysql" {
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
			cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)
		db, err = sql.Open("mysql", dsn)
	} else {
		// SQLite optimization flags to prevent massive file expansion
		dbPath := filepath.Join(dir, "data.db")
		dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=auto_vacuum(FULL)", dbPath)
		db, err = sql.Open("sqlite", dsn)
	}

	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	s := &Storage{db: db, storageType: cfg.StorageType}
	if err := s.initTables(); err != nil {
		return nil, err
	}

	go s.startPruningLoop()
	return s, nil
}

// initTables creates the database schema dynamically based on the driver
func (s *Storage) initTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS pp_groups (
			name TEXT PRIMARY KEY,
			prefix TEXT DEFAULT '',
			weight INTEGER DEFAULT 0
		);`,
		`INSERT OR IGNORE INTO pp_groups (name, prefix, weight) VALUES ('default', '&7[Player] ', 1);`,
		`CREATE TABLE IF NOT EXISTS pp_group_parents (
			group_name TEXT,
			parent_name TEXT,
			PRIMARY KEY (group_name, parent_name)
		);`,
		`CREATE TABLE IF NOT EXISTS pp_group_permissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_name TEXT,
			permission TEXT,
			value INTEGER DEFAULT 1,
			server TEXT DEFAULT 'global',
			UNIQUE(group_name, permission, server)
		);`,
		`CREATE TABLE IF NOT EXISTS pp_users (
			uuid TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			primary_group TEXT DEFAULT 'default'
		);`,
		`CREATE TABLE IF NOT EXISTS pp_user_temp_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT,
			group_name TEXT,
			expiry_time TIMESTAMP NOT NULL,
			UNIQUE(uuid, group_name)
		);`,
		`CREATE TABLE IF NOT EXISTS pp_user_permissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT,
			permission TEXT,
			value INTEGER DEFAULT 1,
			server TEXT DEFAULT 'global',
			UNIQUE(uuid, permission, server)
		);`,
	}

	// Minor adjustments for MySQL auto-increment compatibility if needed
	if s.storageType == "mysql" {
		for i, q := range queries {
			queries[i] = replaceSQLiteWithMySQL(q)
		}
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// replaceSQLiteWithMySQL acts as a small converter for cross-compatibility
func replaceSQLiteWithMySQL(query string) string {
	import strings
	q := strings.ReplaceAll(query, "INTEGER PRIMARY KEY AUTOINCREMENT", "INT AUTO_INCREMENT PRIMARY KEY")
	q = strings.ReplaceAll(query, "TEXT", "VARCHAR(255)")
	return q
}

// Close gracefully shuts down the database connection
func (s *Storage) Close() {
	if s.db != nil {
		s.db.Close()
	}
}

// startPruningLoop runs every minute to clean up expired temp ranks
func (s *Storage) startPruningLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		if s.storageType == "mysql" {
			_, _ = s.db.Exec("DELETE FROM pp_user_temp_groups WHERE expiry_time <= NOW()")
		} else {
			_, _ = s.db.Exec("DELETE FROM pp_user_temp_groups WHERE expiry_time <= datetime('now', 'localtime')")
		}
	}
}

// ==========================================
// User Data Methods
// ==========================================

func (s *Storage) EnsureUser(uuid, username string) error {
	var query string
	if s.storageType == "mysql" {
		query = "INSERT IGNORE INTO pp_users (uuid, username, primary_group) VALUES (?, ?, 'default')"
	} else {
		query = "INSERT OR IGNORE INTO pp_users (uuid, username, primary_group) VALUES (?, ?, 'default')"
	}
	_, err := s.db.Exec(query, uuid, username)
	return err
}

func (s *Storage) SetUserGroup(uuid, group string) error {
	_, err := s.db.Exec("UPDATE pp_users SET primary_group = ? WHERE uuid = ?", group, uuid)
	return err
}

func (s *Storage) AddUserTempGroup(uuid, group string, duration time.Duration) error {
	expiry := time.Now().Add(duration).Format("2006-01-02 15:04:05")
	var query string
	if s.storageType == "mysql" {
		query = `INSERT INTO pp_user_temp_groups (uuid, group_name, expiry_time)
				 VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE expiry_time = ?`
	} else {
		query = `INSERT INTO pp_user_temp_groups (uuid, group_name, expiry_time)
				 VALUES (?, ?, ?) ON CONFLICT(uuid, group_name) DO UPDATE SET expiry_time = ?`
	}
	_, err := s.db.Exec(query, uuid, group, expiry, expiry)
	return err
}

func (s *Storage) RemoveUserGroup(uuid, group string) error {
	_, err := s.db.Exec("DELETE FROM pp_user_temp_groups WHERE uuid = ? AND group_name = ?", uuid, group)
	if err == nil {
		// Fallback check, if it was primary, reset to default
		_, err = s.db.Exec("UPDATE pp_users SET primary_group = 'default' WHERE uuid = ? AND primary_group = ?", uuid, group)
	}
	return err
}

func (s *Storage) SetUserPermission(uuid, perm string, val bool, server string) error {
	vInt := 1
	if !val {
		vInt = 0
	}
	var query string
	if s.storageType == "mysql" {
		query = `INSERT INTO pp_user_permissions (uuid, permission, value, server)
				 VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE value = ?`
	} else {
		query = `INSERT INTO pp_user_permissions (uuid, permission, value, server)
				 VALUES (?, ?, ?, ?) ON CONFLICT(uuid, permission, server) DO UPDATE SET value = ?`
	}
	_, err := s.db.Exec(query, uuid, perm, vInt, server, vInt)
	return err
}

func (s *Storage) UnsetUserPermission(uuid, perm string, server string) error {
	_, err := s.db.Exec("DELETE FROM pp_user_permissions WHERE uuid = ? AND permission = ? AND server = ?", uuid, perm, server)
	return err
}

func (s *Storage) ClearUser(uuid string) error {
	_, err := s.db.Exec("DELETE FROM pp_user_permissions WHERE uuid = ?", uuid)
	if err == nil {
		_, err = s.db.Exec("DELETE FROM pp_user_temp_groups WHERE uuid = ?", uuid)
	}
	if err == nil {
		_, err = s.db.Exec("UPDATE pp_users SET primary_group = 'default' WHERE uuid = ?", uuid)
	}
	return err
}

// ==========================================
// Group Data Methods
// ==========================================

func (s *Storage) CreateGroup(name string) error {
	var query string
	if s.storageType == "mysql" {
		query = "INSERT IGNORE INTO pp_groups (name, prefix, weight) VALUES (?, '', 1)"
	} else {
		query = "INSERT OR IGNORE INTO pp_groups (name, prefix, weight) VALUES (?, '', 1)"
	}
	_, err := s.db.Exec(query, name)
	return err
}

func (s *Storage) DeleteGroup(name string) error {
	if name == "default" {
		return fmt.Errorf("cannot delete the default group")
	}
	_, err := s.db.Exec("DELETE FROM pp_groups WHERE name = ?", name)
	return err
}

func (s *Storage) SetGroupPrefix(name, prefix string) error {
	_, err := s.db.Exec("UPDATE pp_groups SET prefix = ? WHERE name = ?", prefix, name)
	return err
}

func (s *Storage) SetGroupWeight(name string, weight int) error {
	_, err := s.db.Exec("UPDATE pp_groups SET weight = ? WHERE name = ?", weight, name)
	return err
}

func (s *Storage) SetGroupPermission(name, perm string, val bool, server string) error {
	vInt := 1
	if !val {
		vInt = 0
	}
	var query string
	if s.storageType == "mysql" {
		query = `INSERT INTO pp_group_permissions (group_name, permission, value, server)
				 VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE value = ?`
	} else {
		query = `INSERT INTO pp_group_permissions (group_name, permission, value, server)
				 VALUES (?, ?, ?, ?) ON CONFLICT(group_name, permission, server) DO UPDATE SET value = ?`
	}
	_, err := s.db.Exec(query, name, perm, vInt, server, vInt)
	return err
}

func (s *Storage) UnsetGroupPermission(name, perm string, server string) error {
	_, err := s.db.Exec("DELETE FROM pp_group_permissions WHERE group_name = ? AND permission = ? AND server = ?", name, perm, server)
	return err
}

func (s *Storage) AddGroupParent(group, parent string) error {
	var query string
	if s.storageType == "mysql" {
		query = "INSERT IGNORE INTO pp_group_parents (group_name, parent_name) VALUES (?, ?)"
	} else {
		query = "INSERT OR IGNORE INTO pp_group_parents (group_name, parent_name) VALUES (?, ?)"
	}
	_, err := s.db.Exec(query, group, parent)
	return err
}

func (s *Storage) RemoveGroupParent(group, parent string) error {
	_, err := s.db.Exec("DELETE FROM pp_group_parents WHERE group_name = ? AND parent_name = ?", group, parent)
	return err
}