package mysql

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	sqlconn "github.com/cerberauth/scimply/connector/sql"
	"github.com/cerberauth/scimply/resource"
	"github.com/cerberauth/scimply/store"
)

type Store struct {
	cfg     config
	db      *sql.DB
	mapper  *sqlconn.Mapper            // default mapper (JSON mode)
	mappers map[string]*sqlconn.Mapper // per-resource-type mappers (column mode)
}

// DB returns the underlying database connection for custom queries.
func (s *Store) DB() *sql.DB { return s.db }

func New(opts ...Option) (*Store, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.dsn == "" {
		return nil, fmt.Errorf("mysql store: DSN is required (use WithDSN)")
	}

	defaultMapper := sqlconn.NewMapper()
	perTypeMappers := make(map[string]*sqlconn.Mapper)
	for rt, resCfg := range cfg.resourceConfigs {
		if len(resCfg.FieldMappings) == 0 {
			continue
		}
		m := sqlconn.NewMapper()
		for attr, ref := range resCfg.FieldMappings {
			m.ColumnRefs[attr] = ref
		}
		perTypeMappers[rt] = m
	}

	return &Store{
		cfg:     cfg,
		mapper:  defaultMapper,
		mappers: perTypeMappers,
	}, nil
}

func (s *Store) mapperFor(resourceType string) *sqlconn.Mapper {
	if m, ok := s.mappers[strings.ToLower(resourceType)]; ok {
		return m
	}
	return s.mapper
}

func (s *Store) isColumnMode(resourceType string) bool {
	cfg, ok := s.cfg.resourceConfigs[strings.ToLower(resourceType)]
	return ok && len(cfg.FieldMappings) > 0
}

func (s *Store) resCfg(resourceType string) (ResourceTableConfig, bool) {
	cfg, ok := s.cfg.resourceConfigs[strings.ToLower(resourceType)]
	return cfg, ok
}

func (s *Store) Init(ctx context.Context) error {
	db, err := sql.Open("mysql", s.cfg.dsn)
	if err != nil {
		return fmt.Errorf("mysql store: open: %w", err)
	}
	db.SetMaxOpenConns(s.cfg.maxConns)
	if s.cfg.connTimeout > 0 {
		db.SetConnMaxLifetime(s.cfg.connTimeout)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("mysql store: ping: %w", err)
	}
	s.db = db

	if s.cfg.autoMigrate {
		if err := s.migrate(ctx); err != nil {
			return fmt.Errorf("mysql store: migrate: %w", err)
		}
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	prefix := s.cfg.tablePrefix

	// Skip column-mode resource types — the user owns those schemas.
	userCfg := s.cfg.resourceConfigs["user"]
	groupCfg := s.cfg.resourceConfigs["group"]

	if len(userCfg.FieldMappings) == 0 {
		stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
    id            VARCHAR(255) PRIMARY KEY,
    external_id   VARCHAR(255),
    user_name     VARCHAR(255) UNIQUE,
    meta_created  DATETIME(6) NOT NULL,
    meta_last_mod DATETIME(6) NOT NULL,
    meta_version  VARCHAR(255),
    active        TINYINT(1) DEFAULT 1,
    data          JSON NOT NULL,
    schemas       JSON
)`, backtick(prefix+"users"))
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create users table: %w", err)
		}
	}

	if len(groupCfg.FieldMappings) == 0 {
		grpStmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
    id            VARCHAR(255) PRIMARY KEY,
    external_id   VARCHAR(255),
    display_name  VARCHAR(255),
    meta_created  DATETIME(6) NOT NULL,
    meta_last_mod DATETIME(6) NOT NULL,
    meta_version  VARCHAR(255),
    data          JSON NOT NULL,
    schemas       JSON
)`, backtick(prefix+"groups"))
		if _, err := s.db.ExecContext(ctx, grpStmt); err != nil {
			return fmt.Errorf("create groups table: %w", err)
		}
	}
	return nil
}

func (s *Store) Close(_ context.Context) error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *Store) Healthy(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("mysql store: not initialized")
	}
	return s.db.PingContext(ctx)
}

func (s *Store) tableName(resourceType string) string {
	key := strings.ToLower(resourceType)
	if cfg, ok := s.cfg.resourceConfigs[key]; ok && cfg.Table != "" {
		return cfg.Table
	}
	return s.cfg.tablePrefix + strings.ToLower(resourceType) + "s"
}

func (s *Store) Create(ctx context.Context, resourceType string, res *resource.Resource) (*resource.Resource, error) {
	if s.isColumnMode(resourceType) {
		cfg, _ := s.resCfg(resourceType)
		return s.createColumnMode(ctx, resourceType, res, cfg)
	}
	return s.createJSONMode(ctx, resourceType, res)
}

func (s *Store) createJSONMode(ctx context.Context, resourceType string, res *resource.Resource) (*resource.Resource, error) {
	id, err := generateID()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	clone := res.Clone()
	clone.ID = id
	clone.Meta.ResourceType = resourceType
	clone.Meta.Created = now
	clone.Meta.LastModified = now

	data, err := json.Marshal(clone.ToMap())
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %v", store.ErrInternal, err)
	}
	schemasJSON, err := json.Marshal(clone.Schemas)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal schemas: %v", store.ErrInternal, err)
	}

	table := s.tableName(resourceType)
	userName := userNameFromAttrs(clone.Attributes)

	var q string
	var args []interface{}
	switch strings.ToLower(resourceType) {
	case "user":
		q = fmt.Sprintf( //nolint:gosec // G201: Table name is from internal configuration, values are passed as placeholders.
			`INSERT INTO %s (id, external_id, user_name, meta_created, meta_last_mod, meta_version, active, data, schemas)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, backtick(table))
		args = []interface{}{
			id, nullString(clone.ExternalID), nullString(userName),
			now, now, nullString(clone.Meta.Version),
			activeFromAttrs(clone.Attributes), data, schemasJSON,
		}
	default:
		q = fmt.Sprintf( //nolint:gosec // G201: Table name is from internal configuration, values are passed as placeholders.
			`INSERT INTO %s (id, external_id, meta_created, meta_last_mod, meta_version, data, schemas)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`, backtick(table))
		args = []interface{}{
			id, nullString(clone.ExternalID),
			now, now, nullString(clone.Meta.Version), data, schemasJSON,
		}
	}

	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		if isDuplicateEntry(err) {
			return nil, fmt.Errorf("%w: %v", store.ErrConflict, err)
		}
		return nil, fmt.Errorf("%w: insert: %v", store.ErrInternal, err)
	}
	return clone, nil
}

func (s *Store) createColumnMode(ctx context.Context, resourceType string, res *resource.Resource, cfg ResourceTableConfig) (*resource.Resource, error) {
	id, err := generateID()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	clone := res.Clone()
	clone.ID = id
	clone.Meta.ResourceType = resourceType
	clone.Meta.Created = now
	clone.Meta.LastModified = now

	attrMap := sqlconn.ResourceAttrMap(clone)
	attrMap["meta.created"] = now
	attrMap["meta.lastModified"] = now

	priCols, priVals, joinCols, joinVals := splitByTable(cfg, attrMap, "")

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: begin tx: %v", store.ErrInternal, err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := myInsert(ctx, tx, cfg.Table, priCols, priVals); err != nil {
		if isDuplicateEntry(err) {
			return nil, fmt.Errorf("%w: %v", store.ErrConflict, err)
		}
		return nil, fmt.Errorf("%w: insert primary: %v", store.ErrInternal, err)
	}

	for _, jd := range cfg.Joins {
		if jd.ForeignKey == "" {
			continue
		}
		cols := joinCols[jd.Table]
		vals := joinVals[jd.Table]
		if len(cols) == 0 {
			continue
		}
		allCols := append([]string{jd.ForeignKey}, cols...)
		allVals := append([]interface{}{id}, vals...)
		if err := myInsert(ctx, tx, jd.Table, allCols, allVals); err != nil {
			return nil, fmt.Errorf("%w: insert %s: %v", store.ErrInternal, jd.Table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%w: commit: %v", store.ErrInternal, err)
	}
	return clone, nil
}

func (s *Store) Get(ctx context.Context, resourceType string, id string) (*resource.Resource, error) {
	if s.isColumnMode(resourceType) {
		cfg, _ := s.resCfg(resourceType)
		return s.getColumnMode(ctx, resourceType, id, cfg)
	}
	return s.getJSONMode(ctx, resourceType, id)
}

func (s *Store) getJSONMode(ctx context.Context, resourceType string, id string) (*resource.Resource, error) {
	table := s.tableName(resourceType)
	q := fmt.Sprintf( //nolint:gosec // G201: Table name is from internal configuration, value is passed as placeholder.
		`SELECT id, external_id, meta_created, meta_last_mod, meta_version, data, schemas
		 FROM %s WHERE id = ?`, backtick(table))
	row := s.db.QueryRowContext(ctx, q, id)
	return s.scanJSONRow(row, resourceType)
}

func (s *Store) getColumnMode(ctx context.Context, resourceType string, id string, cfg ResourceTableConfig) (*resource.Resource, error) {
	entries, baseQ, idCol := buildColumnSelect(cfg)
	q := baseQ + " WHERE " + idCol + " = ?" //nolint:gosec // G202: Table and column names are from internal configuration, value is passed as placeholder.
	row := s.db.QueryRowContext(ctx, q, id)
	res, err := scanColumnRow(row, entries, resourceType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
		}
		return nil, fmt.Errorf("%w: get: %v", store.ErrInternal, err)
	}
	return res, nil
}

func (s *Store) scanJSONRow(row *sql.Row, resourceType string) (*resource.Resource, error) {
	var (
		id          sql.NullString
		externalID  sql.NullString
		metaCreated time.Time
		metaLastMod time.Time
		metaVersion sql.NullString
		data        []byte
		schemasJSON []byte
	)
	if err := row.Scan(&id, &externalID, &metaCreated, &metaLastMod, &metaVersion, &data, &schemasJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("%w: scan: %v", store.ErrInternal, err)
	}
	r, err := jsonBytesToResource(data, schemasJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", store.ErrInternal, err)
	}
	r.ID = id.String
	if externalID.Valid {
		r.ExternalID = externalID.String
	}
	r.Meta.Created = metaCreated.UTC()
	r.Meta.LastModified = metaLastMod.UTC()
	if metaVersion.Valid {
		r.Meta.Version = metaVersion.String
	}
	r.Meta.ResourceType = resourceType
	return r, nil
}

func (s *Store) List(ctx context.Context, resourceType string, params store.ListParams) (*store.ListResult, error) {
	if s.isColumnMode(resourceType) {
		cfg, _ := s.resCfg(resourceType)
		return s.listColumnMode(ctx, resourceType, params, cfg)
	}
	return s.listJSONMode(ctx, resourceType, params)
}

func (s *Store) listJSONMode(ctx context.Context, resourceType string, params store.ListParams) (*store.ListResult, error) {
	table := s.tableName(resourceType)
	m := s.mapper

	var whereParts []string
	var args []interface{}
	if params.Filter != nil {
		fr := sqlconn.TranslateFilter(params.Filter, m, sqlconn.DialectMySQL, "")
		if fr.Err == nil && fr.Clause != "" {
			whereParts = append(whereParts, fr.Clause)
			args = append(args, fr.Args...)
		}
	}

	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}

	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM %s%s`, backtick(table), where) //nolint:gosec // G201: Table name and where clause are from internal configuration or safe filter translation, values are passed as placeholders.
	var total int
	if err := s.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("%w: count: %v", store.ErrInternal, err)
	}

	startIndex := params.StartIndex
	if startIndex < 1 {
		startIndex = 1
	}
	offset := startIndex - 1
	limit := params.Count
	if limit <= 0 {
		limit = total
	}
	if limit == 0 {
		return &store.ListResult{
			Resources: []*resource.Resource{}, TotalResults: total,
			StartIndex: startIndex, ItemsPerPage: 0,
		}, nil
	}

	orderBy := ""
	if params.SortBy != "" {
		if col, ok := m.ColumnName(params.SortBy); ok {
			dir := "ASC"
			if params.SortOrder == store.SortDescending {
				dir = "DESC"
			}
			orderBy = fmt.Sprintf(" ORDER BY %s %s", backtick(col), dir)
		}
	}

	dataQ := fmt.Sprintf( //nolint:gosec // G201: Table name and query parts are from internal configuration or safe filter translation, values are passed as placeholders.
		`SELECT id, external_id, meta_created, meta_last_mod, meta_version, data, schemas
		 FROM %s%s%s LIMIT ? OFFSET ?`,
		backtick(table), where, orderBy)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, dataQ, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: query: %v", store.ErrInternal, err)
	}
	defer func() { _ = rows.Close() }()

	var resources []*resource.Resource
	for rows.Next() {
		var (
			id, externalID, metaVersion sql.NullString
			metaCreated, metaLastMod    time.Time
			data, schemasJSON           []byte
		)
		if err := rows.Scan(&id, &externalID, &metaCreated, &metaLastMod, &metaVersion, &data, &schemasJSON); err != nil {
			return nil, fmt.Errorf("%w: scan row: %v", store.ErrInternal, err)
		}
		r, err := jsonBytesToResource(data, schemasJSON)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", store.ErrInternal, err)
		}
		r.ID = id.String
		if externalID.Valid {
			r.ExternalID = externalID.String
		}
		r.Meta.Created = metaCreated.UTC()
		r.Meta.LastModified = metaLastMod.UTC()
		if metaVersion.Valid {
			r.Meta.Version = metaVersion.String
		}
		r.Meta.ResourceType = resourceType
		resources = append(resources, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: rows: %v", store.ErrInternal, err)
	}

	needsClientFilter := params.Filter != nil && len(whereParts) == 0
	return &store.ListResult{
		Resources:         resources,
		TotalResults:      total,
		StartIndex:        startIndex,
		ItemsPerPage:      len(resources),
		NeedsClientFilter: needsClientFilter,
	}, nil
}

func (s *Store) listColumnMode(ctx context.Context, resourceType string, params store.ListParams, cfg ResourceTableConfig) (*store.ListResult, error) {
	m := s.mapperFor(resourceType)
	entries, baseQ, _ := buildColumnSelect(cfg)

	var whereParts []string
	var args []interface{}
	if params.Filter != nil {
		result := sqlconn.TranslateFilter(params.Filter, m, sqlconn.DialectMySQL, "")
		if result.Err == nil && result.Clause != "" {
			whereParts = append(whereParts, result.Clause)
			args = append(args, result.Args...)
		}
	}

	fromClause := backtick(cfg.Table) + buildJoinSQL(cfg)
	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " WHERE " + strings.Join(whereParts, " AND ")
	}

	var total int
	countQ := "SELECT COUNT(*) FROM " + fromClause + whereClause
	if err := s.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("%w: count: %v", store.ErrInternal, err)
	}

	startIndex := params.StartIndex
	if startIndex < 1 {
		startIndex = 1
	}

	var sb strings.Builder
	sb.WriteString(baseQ)
	sb.WriteString(whereClause)

	if params.SortBy != "" {
		if ref, ok := m.Ref(params.SortBy); ok {
			dir := "ASC"
			if params.SortOrder == store.SortDescending {
				dir = "DESC"
			}
			col := ref.Column
			if ref.Table != "" {
				col = backtick(ref.Table) + "." + backtick(ref.Column)
			}
			fmt.Fprintf(&sb, " ORDER BY %s %s", col, dir)
		}
	}

	limit := params.Count
	if limit <= 0 {
		limit = total
	}
	sb.WriteString(" LIMIT ?")
	args = append(args, limit)
	sb.WriteString(" OFFSET ?")
	args = append(args, startIndex-1)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("%w: list: %v", store.ErrInternal, err)
	}
	defer func() { _ = rows.Close() }()

	var resources []*resource.Resource
	for rows.Next() {
		res, err := scanColumnRows(rows, entries, resourceType)
		if err != nil {
			return nil, fmt.Errorf("%w: scan: %v", store.ErrInternal, err)
		}
		resources = append(resources, res)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: rows: %v", store.ErrInternal, err)
	}

	return &store.ListResult{
		Resources:    resources,
		TotalResults: total,
		StartIndex:   startIndex,
		ItemsPerPage: len(resources),
	}, nil
}

func (s *Store) Replace(ctx context.Context, resourceType string, id string, res *resource.Resource) (*resource.Resource, error) {
	if s.isColumnMode(resourceType) {
		cfg, _ := s.resCfg(resourceType)
		return s.replaceColumnMode(ctx, resourceType, id, res, cfg)
	}
	return s.replaceJSONMode(ctx, resourceType, id, res)
}

func (s *Store) replaceJSONMode(ctx context.Context, resourceType string, id string, res *resource.Resource) (*resource.Resource, error) {
	existing, err := s.getJSONMode(ctx, resourceType, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	clone := res.Clone()
	clone.ID = id
	clone.Meta.Created = existing.Meta.Created
	clone.Meta.ResourceType = resourceType
	clone.Meta.LastModified = now

	data, err := json.Marshal(clone.ToMap())
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %v", store.ErrInternal, err)
	}
	schemasJSON, err := json.Marshal(clone.Schemas)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal schemas: %v", store.ErrInternal, err)
	}

	table := s.tableName(resourceType)

	var q string
	var args []interface{}
	switch strings.ToLower(resourceType) {
	case "user":
		q = fmt.Sprintf( //nolint:gosec // G201: Table name is from internal configuration, values are passed as placeholders.
			`UPDATE %s SET external_id=?, user_name=?, meta_last_mod=?, meta_version=?, active=?, data=?, schemas=?
			 WHERE id=?`, backtick(table))
		args = []interface{}{
			nullString(clone.ExternalID),
			nullString(userNameFromAttrs(clone.Attributes)),
			now, nullString(clone.Meta.Version),
			activeFromAttrs(clone.Attributes), data, schemasJSON, id,
		}
	default:
		q = fmt.Sprintf( //nolint:gosec // G201: Table name is from internal configuration, values are passed as placeholders.
			`UPDATE %s SET external_id=?, meta_last_mod=?, meta_version=?, data=?, schemas=?
			 WHERE id=?`, backtick(table))
		args = []interface{}{
			nullString(clone.ExternalID),
			now, nullString(clone.Meta.Version), data, schemasJSON, id,
		}
	}

	result, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		if isDuplicateEntry(err) {
			return nil, fmt.Errorf("%w: %v", store.ErrConflict, err)
		}
		return nil, fmt.Errorf("%w: update: %v", store.ErrInternal, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
	}
	return clone, nil
}

func (s *Store) replaceColumnMode(ctx context.Context, resourceType string, id string, res *resource.Resource, cfg ResourceTableConfig) (*resource.Resource, error) {
	existing, err := s.getColumnMode(ctx, resourceType, id, cfg)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	clone := res.Clone()
	clone.ID = id
	clone.Meta.Created = existing.Meta.Created
	clone.Meta.ResourceType = resourceType
	clone.Meta.LastModified = now

	attrMap := sqlconn.ResourceAttrMap(clone)
	attrMap["meta.created"] = existing.Meta.Created
	attrMap["meta.lastModified"] = now

	idColName := primaryIDCol(cfg)
	priCols, priVals, joinCols, joinVals := splitByTable(cfg, attrMap, idColName)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: begin tx: %v", store.ErrInternal, err)
	}
	defer func() { _ = tx.Rollback() }()

	found, err := myUpdate(ctx, tx, cfg.Table, idColName, priCols, priVals, id)
	if err != nil {
		if isDuplicateEntry(err) {
			return nil, fmt.Errorf("%w: %v", store.ErrConflict, err)
		}
		return nil, fmt.Errorf("%w: update primary: %v", store.ErrInternal, err)
	}
	if !found {
		return nil, fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
	}

	for _, jd := range cfg.Joins {
		if jd.ForeignKey == "" {
			continue
		}
		cols := joinCols[jd.Table]
		vals := joinVals[jd.Table]
		if len(cols) == 0 {
			continue
		}
		if err := myUpsertJoin(ctx, tx, jd, cols, vals, id); err != nil {
			return nil, fmt.Errorf("%w: upsert %s: %v", store.ErrInternal, jd.Table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%w: commit: %v", store.ErrInternal, err)
	}
	return clone, nil
}

func (s *Store) Patch(ctx context.Context, resourceType string, id string, ops []resource.PatchOp) (*resource.Resource, error) {
	existing, err := s.Get(ctx, resourceType, id)
	if err != nil {
		return nil, err
	}
	patched, err := resource.ApplyPatch(existing, ops)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", store.ErrBadPatch, err)
	}
	return s.Replace(ctx, resourceType, id, patched)
}

func (s *Store) Delete(ctx context.Context, resourceType string, id string) error {
	if s.isColumnMode(resourceType) {
		cfg, _ := s.resCfg(resourceType)
		return s.deleteColumnMode(ctx, resourceType, id, cfg)
	}
	return s.deleteJSONMode(ctx, resourceType, id)
}

func (s *Store) deleteJSONMode(ctx context.Context, resourceType string, id string) error {
	table := s.tableName(resourceType)
	result, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, backtick(table)), id) //nolint:gosec // G201: Table name is from internal configuration, value is passed as placeholder.
	if err != nil {
		return fmt.Errorf("%w: delete: %v", store.ErrInternal, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
	}
	return nil
}

func (s *Store) deleteColumnMode(ctx context.Context, resourceType string, id string, cfg ResourceTableConfig) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: begin tx: %v", store.ErrInternal, err)
	}
	defer func() { _ = tx.Rollback() }()

	idColName := primaryIDCol(cfg)
	for i := len(cfg.Joins) - 1; i >= 0; i-- {
		jd := cfg.Joins[i]
		if !jd.DeleteJoin || jd.ForeignKey == "" {
			continue
		}
		q := fmt.Sprintf("DELETE FROM %s WHERE %s = ?", backtick(jd.Table), backtick(jd.ForeignKey)) //nolint:gosec // G201: Table and column names are from internal configuration, value is passed as placeholder.
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return fmt.Errorf("%w: delete %s: %v", store.ErrInternal, jd.Table, err)
		}
	}

	result, err := tx.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM %s WHERE %s = ?", backtick(cfg.Table), backtick(idColName)), id)
	if err != nil {
		return fmt.Errorf("%w: delete: %v", store.ErrInternal, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: commit: %v", store.ErrInternal, err)
	}
	return nil
}

type fieldEntry struct {
	scimAttr string
	col      string // backtick-quoted table.column
}

func buildColumnSelect(cfg ResourceTableConfig) ([]fieldEntry, string, string) {
	entries := orderedEntries(cfg)
	cols := make([]string, len(entries))
	for i, e := range entries {
		cols[i] = e.col
	}
	from := backtick(cfg.Table) + buildJoinSQL(cfg)
	q := "SELECT " + strings.Join(cols, ", ") + " FROM " + from
	idCol := backtick(cfg.Table) + "." + backtick(primaryIDCol(cfg))
	return entries, q, idCol
}

func orderedEntries(cfg ResourceTableConfig) []fieldEntry {
	entries := make([]fieldEntry, 0, len(cfg.FieldMappings))
	for attr, ref := range cfg.FieldMappings {
		tbl := ref.Table
		if tbl == "" {
			tbl = cfg.Table
		}
		entries = append(entries, fieldEntry{
			scimAttr: attr,
			col:      backtick(tbl) + "." + backtick(ref.Column),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].scimAttr < entries[j].scimAttr })
	return entries
}

func buildJoinSQL(cfg ResourceTableConfig) string {
	if len(cfg.Joins) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, jd := range cfg.Joins {
		jt := jd.JoinType
		if jt == "" {
			jt = "LEFT"
		}
		fmt.Fprintf(&sb, " %s JOIN %s", jt, backtick(jd.Table))
		if jd.Alias != "" {
			sb.WriteString(" " + backtick(jd.Alias))
		}
		if jd.Condition != "" {
			sb.WriteString(" ON " + jd.Condition)
		}
	}
	return sb.String()
}

func primaryIDCol(cfg ResourceTableConfig) string {
	if ref, ok := cfg.FieldMappings["id"]; ok && ref.Column != "" {
		return ref.Column
	}
	return "id"
}

func splitByTable(cfg ResourceTableConfig, attrMap map[string]interface{}, excludeCol string) (
	priCols []string, priVals []interface{},
	joinCols map[string][]string, joinVals map[string][]interface{},
) {
	joinCols = make(map[string][]string)
	joinVals = make(map[string][]interface{})
	for attr, ref := range cfg.FieldMappings {
		if ref.Column == excludeCol && (ref.Table == "" || ref.Table == cfg.Table) {
			continue
		}
		val := attrMap[attr]
		if ref.Table == "" || ref.Table == cfg.Table {
			priCols = append(priCols, ref.Column)
			priVals = append(priVals, val)
		} else {
			joinCols[ref.Table] = append(joinCols[ref.Table], ref.Column)
			joinVals[ref.Table] = append(joinVals[ref.Table], val)
		}
	}
	return
}

func scanColumnRow(row *sql.Row, entries []fieldEntry, resourceType string) (*resource.Resource, error) {
	vals := make([]interface{}, len(entries))
	ptrs := make([]interface{}, len(entries))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := row.Scan(ptrs...); err != nil {
		return nil, err
	}
	attrMap := make(map[string]interface{}, len(entries))
	for i, e := range entries {
		attrMap[e.scimAttr] = vals[i]
	}
	return sqlconn.ReconstructResource(attrMap, resourceType), nil
}

func scanColumnRows(rows *sql.Rows, entries []fieldEntry, resourceType string) (*resource.Resource, error) {
	vals := make([]interface{}, len(entries))
	ptrs := make([]interface{}, len(entries))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	attrMap := make(map[string]interface{}, len(entries))
	for i, e := range entries {
		attrMap[e.scimAttr] = vals[i]
	}
	return sqlconn.ReconstructResource(attrMap, resourceType), nil
}

func myInsert(ctx context.Context, tx *sql.Tx, table string, cols []string, vals []interface{}) error {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = backtick(c)
	}
	phs := make([]string, len(vals))
	for i := range vals {
		phs[i] = "?"
	}
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", //nolint:gosec // G201: Table and column names are from internal configuration, values are passed as placeholders.
		backtick(table), strings.Join(quoted, ", "), strings.Join(phs, ", "))
	_, err := tx.ExecContext(ctx, q, vals...)
	return err
}

func myUpdate(ctx context.Context, tx *sql.Tx, table, idCol string, cols []string, vals []interface{}, id string) (bool, error) {
	if len(cols) == 0 {
		return true, nil
	}
	setClauses := make([]string, len(cols))
	for i, c := range cols {
		setClauses[i] = backtick(c) + " = ?"
	}
	q := fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?", //nolint:gosec // G201: Table and column names are from internal configuration, values are passed as placeholders.
		backtick(table), strings.Join(setClauses, ", "), backtick(idCol))
	vals = append(vals, id)
	result, err := tx.ExecContext(ctx, q, vals...)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// myUpsertJoin uses INSERT … ON DUPLICATE KEY UPDATE for joined-table upserts.
// Requires a unique/primary-key constraint on jd.ForeignKey in the joined table.
func myUpsertJoin(ctx context.Context, tx *sql.Tx, jd JoinDef, cols []string, vals []interface{}, id string) error {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = backtick(c)
	}
	allCols := append([]string{backtick(jd.ForeignKey)}, quoted...)
	allVals := append([]interface{}{id}, vals...)

	phs := make([]string, len(allVals))
	for i := range allVals {
		phs[i] = "?"
	}
	setClauses := make([]string, len(quoted))
	for i, c := range quoted {
		setClauses[i] = fmt.Sprintf("%s = VALUES(%s)", c, c)
	}
	q := fmt.Sprintf( //nolint:gosec // G201: Table and column names are from internal configuration, values are passed as placeholders.
		"INSERT INTO %s (%s) VALUES (%s) ON DUPLICATE KEY UPDATE %s",
		backtick(jd.Table),
		strings.Join(allCols, ", "),
		strings.Join(phs, ", "),
		strings.Join(setClauses, ", "),
	)
	_, err := tx.ExecContext(ctx, q, allVals...)
	return err
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("%w: generate id: %v", store.ErrInternal, err)
	}
	return hex.EncodeToString(b), nil
}

func jsonBytesToResource(data, schemasJSON []byte) (*resource.Resource, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal data: %w", err)
	}
	r := resource.FromMap(m)
	if len(schemasJSON) > 0 {
		var schemas []string
		if err := json.Unmarshal(schemasJSON, &schemas); err == nil {
			r.Schemas = schemas
		}
	}
	return r, nil
}

func backtick(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func userNameFromAttrs(attrs map[string]interface{}) string {
	for k, v := range attrs {
		if strings.EqualFold(k, "userName") {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func activeFromAttrs(attrs map[string]interface{}) interface{} {
	for k, v := range attrs {
		if strings.EqualFold(k, "active") {
			return v
		}
	}
	return true
}

func isDuplicateEntry(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Duplicate entry") ||
		strings.Contains(err.Error(), "1062")
}
