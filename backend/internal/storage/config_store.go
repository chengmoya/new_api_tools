package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/new-api-tools/backend/internal/cache"
	"github.com/new-api-tools/backend/internal/config"
	"github.com/new-api-tools/backend/internal/logger"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	BackendToolsDB = "tools_db"
	BackendCache   = "cache"
)

// ConfigRecord represents one persisted configuration item.
type ConfigRecord struct {
	Key         string      `json:"key" db:"config_key"`
	Value       interface{} `json:"value"`
	Description string      `json:"description" db:"description"`
	UpdatedAt   time.Time   `json:"updated_at" db:"updated_at"`
	Backend     string      `json:"backend"`
}

type dbConfigRecord struct {
	Key         string    `db:"config_key"`
	Value       string    `db:"config_value"`
	Description string    `db:"description"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// ConfigStore persists configurable items into the tools database,
// while keeping cache-based compatibility for historical data.
type ConfigStore struct {
	cfg    *config.Config
	db     *sqlx.DB
	driver string
}

var globalStore *ConfigStore

func NewConfigStore() *ConfigStore {
	cfg := config.Get()
	store := &ConfigStore{
		cfg:    cfg,
		driver: cfg.ToolsDriverName(),
	}

	dsn := cfg.ToolsDSN()
	if dsn == "" {
		logger.L.Warn("TOOLS_SQL_DSN 与 SQL_DSN 均未配置，配置项将仅保留兼容缓存模式")
		return store
	}

	db, err := sqlx.Connect(store.driver, dsn)
	if err != nil {
		logger.L.Warn("tools 配置库连接失败，回退兼容缓存模式: " + err.Error())
		return store
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(3 * time.Minute)
	store.db = db
	return store
}

func Init() error {
	globalStore = NewConfigStore()
	if !globalStore.DatabaseEnabled() {
		logger.L.System("配置存储后端: cache(兼容模式)")
		return nil
	}
	if err := globalStore.ensureTables(); err != nil {
		logger.L.Warn("tools 配置表初始化失败，回退兼容缓存模式: " + err.Error())
		_ = globalStore.Close()
		globalStore.db = nil
		return nil
	}
	logger.L.System("配置存储后端: tools_db")
	return nil
}

func GetConfigStore() *ConfigStore {
	if globalStore == nil {
		globalStore = NewConfigStore()
	}
	return globalStore
}

func Close() error {
	if globalStore != nil {
		return globalStore.Close()
	}
	return nil
}

func (s *ConfigStore) Close() error {
	if s != nil && s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *ConfigStore) DatabaseEnabled() bool {
	return s != nil && s.db != nil
}

func (s *ConfigStore) ActiveBackend() string {
	if s.DatabaseEnabled() {
		return BackendToolsDB
	}
	return BackendCache
}

func (s *ConfigStore) ensureTables() error {
	if !s.DatabaseEnabled() {
		return nil
	}

	if s.driver == "pgx" {
		_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS tool_configs (
	config_key TEXT PRIMARY KEY,
	config_value TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`)
		return err
	}

	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS tool_configs (
	config_key VARCHAR(191) PRIMARY KEY,
	config_value LONGTEXT NOT NULL,
	description VARCHAR(255) NOT NULL DEFAULT '',
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	return err
}

func (s *ConfigStore) dbGetRecord(key string) (*dbConfigRecord, error) {
	if !s.DatabaseEnabled() {
		return nil, nil
	}

	var row dbConfigRecord
	query := `SELECT config_key, config_value, description, updated_at FROM tool_configs WHERE config_key = ?`
	if s.driver == "pgx" {
		query = `SELECT config_key, config_value, description, updated_at FROM tool_configs WHERE config_key = $1`
	}
	if err := s.db.Get(&row, query, key); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no rows") {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (s *ConfigStore) dbSetRecord(key string, value interface{}, description string) error {
	if !s.DatabaseEnabled() {
		return nil
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if s.driver == "pgx" {
		_, err = s.db.Exec(`
INSERT INTO tool_configs (config_key, config_value, description, updated_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (config_key)
DO UPDATE SET config_value = EXCLUDED.config_value, description = EXCLUDED.description, updated_at = NOW()`, key, string(payload), description)
		return err
	}

	_, err = s.db.Exec(`
INSERT INTO tool_configs (config_key, config_value, description, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
	config_value = VALUES(config_value),
	description = VALUES(description),
	updated_at = CURRENT_TIMESTAMP`, key, string(payload), description)
	return err
}

func (s *ConfigStore) dbDeleteRecord(key string) error {
	if !s.DatabaseEnabled() {
		return nil
	}
	query := `DELETE FROM tool_configs WHERE config_key = ?`
	if s.driver == "pgx" {
		query = `DELETE FROM tool_configs WHERE config_key = $1`
	}
	_, err := s.db.Exec(query, key)
	return err
}

func (s *ConfigStore) GetJSON(key string, dest interface{}) (bool, error) {
	if rec, err := s.dbGetRecord(key); err != nil {
		return false, err
	} else if rec != nil {
		return true, json.Unmarshal([]byte(rec.Value), dest)
	}

	found, err := cache.Get().GetJSON(key, dest)
	if err != nil || !found {
		return found, err
	}
	_ = s.dbSetRecord(key, dest, "migrated from legacy cache")
	return true, nil
}

func (s *ConfigStore) SetJSON(key string, value interface{}, description string) error {
	if err := s.dbSetRecord(key, value, description); err != nil {
		return err
	}
	return cache.Get().Set(key, value, 0)
}

func (s *ConfigStore) DeleteJSON(key string) error {
	if err := s.dbDeleteRecord(key); err != nil {
		return err
	}
	return cache.Get().Delete(key)
}

func (s *ConfigStore) GetValue(key string) (interface{}, bool, error) {
	if rec, err := s.dbGetRecord(key); err != nil {
		return nil, false, err
	} else if rec != nil {
		var value interface{}
		if err := json.Unmarshal([]byte(rec.Value), &value); err != nil {
			return nil, false, err
		}
		return value, true, nil
	}

	legacy, err := cache.Get().HashGet("app:config", key)
	if err == nil && legacy != "" {
		var value interface{}
		if json.Unmarshal([]byte(legacy), &value) != nil {
			value = legacy
		}
		_ = s.dbSetRecord(key, value, "migrated from legacy hash")
		return value, true, nil
	}

	return nil, false, nil
}

func (s *ConfigStore) SetValue(key string, value interface{}, description string) error {
	if err := s.dbSetRecord(key, value, description); err != nil {
		return err
	}
	return cache.Get().HashSet("app:config", key, value)
}

func (s *ConfigStore) DeleteValue(key string) error {
	if err := s.dbDeleteRecord(key); err != nil {
		return err
	}
	_, err := cache.Get().HashDelete("app:config", key)
	return err
}

func (s *ConfigStore) GetAll() (map[string]interface{}, error) {
	result := make(map[string]interface{})
	if s.DatabaseEnabled() {
		rows := make([]dbConfigRecord, 0)
		query := `SELECT config_key, config_value, description, updated_at FROM tool_configs ORDER BY config_key ASC`
		if err := s.db.Select(&rows, query); err != nil {
			return nil, err
		}
		for _, row := range rows {
			var value interface{}
			if json.Unmarshal([]byte(row.Value), &value) != nil {
				value = row.Value
			}
			result[row.Key] = value
		}
		return result, nil
	}

	legacy, err := cache.Get().GetAllHashFields("app:config")
	if err != nil {
		return result, nil
	}
	for key, raw := range legacy {
		var value interface{}
		if json.Unmarshal([]byte(raw), &value) != nil {
			value = raw
		}
		result[key] = value
	}
	return result, nil
}

func (s *ConfigStore) GetRecord(key string) (*ConfigRecord, bool, error) {
	if rec, err := s.dbGetRecord(key); err != nil {
		return nil, false, err
	} else if rec != nil {
		var value interface{}
		if err := json.Unmarshal([]byte(rec.Value), &value); err != nil {
			return nil, false, err
		}
		return &ConfigRecord{
			Key:         rec.Key,
			Value:       value,
			Description: rec.Description,
			UpdatedAt:   rec.UpdatedAt,
			Backend:     BackendToolsDB,
		}, true, nil
	}

	value, found, err := s.GetValue(key)
	if err != nil || !found {
		return nil, found, err
	}
	return &ConfigRecord{
		Key:     key,
		Value:   value,
		Backend: BackendCache,
		UpdatedAt: time.Now(),
	}, true, nil
}

func (s *ConfigStore) String() string {
	return fmt.Sprintf("ConfigStore{backend=%s}", s.ActiveBackend())
}
