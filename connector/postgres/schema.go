package postgres

import (
	"context"
	"fmt"
	"strings"

	sqlconn "github.com/cerberauth/scimply/connector/sql"
	"github.com/cerberauth/scimply/schema"
)

func (s *Store) tableName(resourceType string) string {
	key := strings.ToLower(resourceType)
	if cfg, ok := s.cfg.resourceConfigs[key]; ok && cfg.Table != "" {
		return cfg.Table
	}
	return s.cfg.tablePrefix + strings.ToLower(resourceType) + "s"
}

func (s *Store) migrate(ctx context.Context) error {
	if s.cfg.registry == nil {

		return nil
	}

	for _, rt := range s.cfg.registry.ResourceTypes() {
		// Skip column-mode resources — the user owns that schema.
		if cfg, ok := s.cfg.resourceConfigs[strings.ToLower(rt.Name)]; ok && len(cfg.FieldMappings) > 0 {
			continue
		}

		sc, ok := s.cfg.registry.SchemaByID(rt.Schema)
		if !ok {

			sc = &schema.Schema{}
		}
		if err := s.migrateResourceType(ctx, rt, sc); err != nil {
			return fmt.Errorf("migrate %s: %w", rt.Name, err)
		}
	}
	return nil
}

func (s *Store) migrateResourceType(ctx context.Context, rt *schema.ResourceType, _ *schema.Schema) error {
	table := s.tableName(rt.Name)

	userNameConstraint := ""
	if strings.EqualFold(rt.Name, "User") {
		userNameConstraint = " UNIQUE"
	}

	ddl := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    id            TEXT NOT NULL PRIMARY KEY,
    external_id   TEXT,
    user_name     TEXT%s,
    meta_created  TIMESTAMPTZ NOT NULL DEFAULT now(),
    meta_last_mod TIMESTAMPTZ NOT NULL DEFAULT now(),
    meta_version  TEXT,
    active        BOOLEAN NOT NULL DEFAULT true,
    data          JSONB NOT NULL,
    schemas       TEXT[]
)`, quoteIdent(table), userNameConstraint)

	_, err := s.pool.Exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("create table %s: %w", table, err)
	}

	idxName := table + "_data_gin"
	idxDDL := fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS %s ON %s USING GIN (data)`,
		quoteIdent(idxName),
		quoteIdent(table),
	)
	if _, err := s.pool.Exec(ctx, idxDDL); err != nil {

		_ = err
	}

	return nil
}

func GenerateTableDDL(rt *schema.ResourceType, sc *schema.Schema, tablePrefix string) sqlconn.TableDef {
	return sqlconn.GenerateTableDef(rt, sc, tablePrefix, sqlconn.DialectPostgres)
}
