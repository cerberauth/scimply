package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlconn "github.com/cerberauth/scimply/connector/sql"
	"github.com/cerberauth/scimply/resource"
	"github.com/cerberauth/scimply/store"
)

type Store struct {
	cfg     config
	pool    *pgxpool.Pool
	mapper  *sqlconn.Mapper            // default mapper (JSON mode)
	mappers map[string]*sqlconn.Mapper // per-resource-type mappers (column mode)
}

var _ store.ResourceStore = (*Store)(nil)

// Pool returns the underlying connection pool for custom queries.
// Returns nil before Init is called.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func New(opts ...Option) (*Store, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.dsn == "" {
		return nil, fmt.Errorf("postgres store: DSN is required (use WithDSN)")
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
	poolCfg, err := pgxpool.ParseConfig(s.cfg.dsn)
	if err != nil {
		return fmt.Errorf("postgres store: parse DSN: %w", err)
	}
	poolCfg.MaxConns = int32(s.cfg.maxConns)
	poolCfg.MinConns = int32(s.cfg.minConns)
	poolCfg.MaxConnIdleTime = s.cfg.connTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("postgres store: create pool: %w", err)
	}
	s.pool = pool

	if s.cfg.autoMigrate {
		if err := s.migrate(ctx); err != nil {
			pool.Close()
			s.pool = nil
			return fmt.Errorf("postgres store: migration failed: %w", err)
		}
	}
	return nil
}

func (s *Store) Close(_ context.Context) error {
	if s.pool != nil {
		s.pool.Close()
		s.pool = nil
	}
	return nil
}

func (s *Store) Healthy(ctx context.Context) error {
	if s.pool == nil {
		return fmt.Errorf("postgres store: pool not initialised")
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("postgres store: acquire connection: %w", err)
	}
	defer conn.Release()
	return conn.Ping(ctx)
}

// ─── Create ──────────────────────────────────────────────────────────────────

func (s *Store) Create(ctx context.Context, resourceType string, res *resource.Resource) (*resource.Resource, error) {
	if s.isColumnMode(resourceType) {
		cfg, _ := s.resCfg(resourceType)
		return s.createColumnMode(ctx, resourceType, res, cfg)
	}
	return s.createJSONMode(ctx, resourceType, res)
}

func (s *Store) createJSONMode(ctx context.Context, resourceType string, res *resource.Resource) (*resource.Resource, error) {
	id, err := generateUUID()
	if err != nil {
		return nil, fmt.Errorf("%w: generate id: %v", store.ErrInternal, err)
	}

	now := time.Now().UTC()
	clone := res.Clone()
	clone.ID = id
	clone.Meta.ResourceType = resourceType
	clone.Meta.Created = now
	clone.Meta.LastModified = now

	data, err := json.Marshal(clone.ToMap())
	if err != nil {
		return nil, fmt.Errorf("%w: marshal resource: %v", store.ErrInternal, err)
	}

	table := s.tableName(resourceType)
	query := fmt.Sprintf(`
		INSERT INTO %s
			(id, external_id, user_name, meta_created, meta_last_mod, meta_version, active, data, schemas)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, quoteIdent(table))

	_, err = s.pool.Exec(ctx, query,
		clone.ID,
		nullableString(clone.ExternalID),
		nullableString(extractUserName(clone)),
		clone.Meta.Created,
		clone.Meta.LastModified,
		nullableString(clone.Meta.Version),
		extractActive(clone),
		data,
		clone.Schemas,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: %v", store.ErrConflict, err)
		}
		return nil, fmt.Errorf("%w: insert: %v", store.ErrInternal, err)
	}
	return clone, nil
}

func (s *Store) createColumnMode(ctx context.Context, resourceType string, res *resource.Resource, cfg ResourceTableConfig) (*resource.Resource, error) {
	id, err := generateUUID()
	if err != nil {
		return nil, fmt.Errorf("%w: generate id: %v", store.ErrInternal, err)
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: begin tx: %v", store.ErrInternal, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := pgInsert(ctx, tx, cfg.Table, priCols, priVals); err != nil {
		if isUniqueViolation(err) {
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
		if err := pgInsert(ctx, tx, jd.Table, allCols, allVals); err != nil {
			return nil, fmt.Errorf("%w: insert %s: %v", store.ErrInternal, jd.Table, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("%w: commit: %v", store.ErrInternal, err)
	}
	return clone, nil
}

// ─── Get ─────────────────────────────────────────────────────────────────────

func (s *Store) Get(ctx context.Context, resourceType string, id string) (*resource.Resource, error) {
	if s.isColumnMode(resourceType) {
		cfg, _ := s.resCfg(resourceType)
		return s.getColumnMode(ctx, resourceType, id, cfg)
	}
	return s.getJSONMode(ctx, resourceType, id)
}

func (s *Store) getJSONMode(ctx context.Context, resourceType string, id string) (*resource.Resource, error) {
	table := s.tableName(resourceType)
	query := fmt.Sprintf(`SELECT data FROM %s WHERE id = $1`, quoteIdent(table))
	row := s.pool.QueryRow(ctx, query, id)
	res, err := scanJSONRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
		}
		return nil, fmt.Errorf("%w: get: %v", store.ErrInternal, err)
	}
	return res, nil
}

func (s *Store) getColumnMode(ctx context.Context, resourceType string, id string, cfg ResourceTableConfig) (*resource.Resource, error) {
	entries, baseQ, idCol := buildColumnSelect(cfg)
	q := baseQ + " WHERE " + idCol + " = $1"
	row := s.pool.QueryRow(ctx, q, id)
	res, err := scanColumnRow(row, entries, resourceType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
		}
		return nil, fmt.Errorf("%w: get: %v", store.ErrInternal, err)
	}
	return res, nil
}

// ─── List ────────────────────────────────────────────────────────────────────

func (s *Store) List(ctx context.Context, resourceType string, params store.ListParams) (*store.ListResult, error) {
	if s.isColumnMode(resourceType) {
		cfg, _ := s.resCfg(resourceType)
		return s.listColumnMode(ctx, resourceType, params, cfg)
	}
	return s.listJSONMode(ctx, resourceType, params)
}

func (s *Store) listJSONMode(ctx context.Context, resourceType string, params store.ListParams) (*store.ListResult, error) {
	countQ, countArgs, err := s.buildCountQuery(resourceType, params.Filter)
	needsClientFilter := err != nil

	var totalResults int
	if !needsClientFilter {
		if scanErr := s.pool.QueryRow(ctx, countQ, countArgs...).Scan(&totalResults); scanErr != nil {
			return nil, fmt.Errorf("%w: count query: %v", store.ErrInternal, scanErr)
		}
	}

	listQ, listArgs, err := s.buildListQuery(resourceType, params)
	if err != nil {
		listQ, listArgs, err = s.buildFallbackQuery(resourceType)
		if err != nil {
			return nil, err
		}
		needsClientFilter = true
	}

	rows, err := s.pool.Query(ctx, listQ, listArgs...)
	if err != nil {
		return nil, fmt.Errorf("%w: list query: %v", store.ErrInternal, err)
	}
	defer rows.Close()

	var resources []*resource.Resource
	for rows.Next() {
		res, err := scanJSONRow(rows)
		if err != nil {
			return nil, fmt.Errorf("%w: scan row: %v", store.ErrInternal, err)
		}
		resources = append(resources, res)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: rows iteration: %v", store.ErrInternal, err)
	}

	if needsClientFilter {
		var filtered []*resource.Resource
		for _, r := range resources {
			if params.Filter == nil || resource.EvalFilter(params.Filter, r) {
				filtered = append(filtered, r)
			}
		}
		if params.SortBy != "" {
			order := resource.SortAscending
			if params.SortOrder == store.SortDescending {
				order = resource.SortDescending
			}
			resource.SortResources(filtered, params.SortBy, order)
		}
		totalResults = len(filtered)
		startIndex := params.StartIndex
		if startIndex < 1 {
			startIndex = 1
		}
		start := startIndex - 1
		if start > totalResults {
			start = totalResults
		}
		pageSize := params.Count
		if pageSize <= 0 {
			pageSize = totalResults
		}
		end := start + pageSize
		if end > totalResults {
			end = totalResults
		}
		page := filtered[start:end]
		return &store.ListResult{
			Resources:    page,
			TotalResults: totalResults,
			StartIndex:   startIndex,
			ItemsPerPage: len(page),
		}, nil
	}

	startIndex := params.StartIndex
	if startIndex < 1 {
		startIndex = 1
	}
	return &store.ListResult{
		Resources:    resources,
		TotalResults: totalResults,
		StartIndex:   startIndex,
		ItemsPerPage: len(resources),
	}, nil
}

func (s *Store) listColumnMode(ctx context.Context, resourceType string, params store.ListParams, cfg ResourceTableConfig) (*store.ListResult, error) {
	m := s.mapperFor(resourceType)
	entries, baseQ, _ := buildColumnSelect(cfg)

	var whereParts []string
	var args []interface{}
	if params.Filter != nil {
		result := sqlconn.TranslateFilter(params.Filter, m, sqlconn.DialectPostgres, "")
		if result.Err == nil && result.Clause != "" {
			whereParts = append(whereParts, result.Clause)
			args = append(args, result.Args...)
		}
	}

	fromClause := quoteIdent(cfg.Table) + buildJoinSQL(cfg)
	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " WHERE " + strings.Join(whereParts, " AND ")
	}

	var total int
	countQ := "SELECT COUNT(*) FROM " + fromClause + whereClause
	if err := s.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("%w: count: %v", store.ErrInternal, err)
	}

	startIndex := params.StartIndex
	if startIndex < 1 {
		startIndex = 1
	}

	var sb strings.Builder
	sb.WriteString(baseQ)
	sb.WriteString(whereClause)
	argIdx := len(args) + 1

	if params.SortBy != "" {
		if ref, ok := m.Ref(params.SortBy); ok {
			dir := "ASC"
			if params.SortOrder == store.SortDescending {
				dir = "DESC"
			}
			col := ref.Column
			if ref.Table != "" {
				col = quoteIdent(ref.Table) + "." + quoteIdent(ref.Column)
			}
			fmt.Fprintf(&sb, " ORDER BY %s %s", col, dir)
		}
	}
	if params.Count > 0 {
		fmt.Fprintf(&sb, " LIMIT $%d", argIdx)
		args = append(args, params.Count)
		argIdx++
	}
	if startIndex > 1 {
		fmt.Fprintf(&sb, " OFFSET $%d", argIdx)
		args = append(args, startIndex-1)
	}

	rows, err := s.pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("%w: list: %v", store.ErrInternal, err)
	}
	defer rows.Close()

	var resources []*resource.Resource
	for rows.Next() {
		res, err := scanColumnRow(rows, entries, resourceType)
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

// ─── Replace ─────────────────────────────────────────────────────────────────

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
		return nil, fmt.Errorf("%w: marshal resource: %v", store.ErrInternal, err)
	}

	table := s.tableName(resourceType)
	query := fmt.Sprintf(`
		UPDATE %s SET
			external_id   = $2,
			user_name     = $3,
			meta_last_mod = $4,
			meta_version  = $5,
			active        = $6,
			data          = $7,
			schemas       = $8
		WHERE id = $1
	`, quoteIdent(table))

	tag, err := s.pool.Exec(ctx, query,
		id,
		nullableString(clone.ExternalID),
		nullableString(extractUserName(clone)),
		clone.Meta.LastModified,
		nullableString(clone.Meta.Version),
		extractActive(clone),
		data,
		clone.Schemas,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: %v", store.ErrConflict, err)
		}
		return nil, fmt.Errorf("%w: update: %v", store.ErrInternal, err)
	}
	if tag.RowsAffected() == 0 {
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

	// Split fields; exclude "id" from SET clauses (used in WHERE instead)
	idColName := primaryIDCol(cfg)
	priCols, priVals, joinCols, joinVals := splitByTable(cfg, attrMap, idColName)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: begin tx: %v", store.ErrInternal, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	found, err := pgUpdate(ctx, tx, cfg.Table, idColName, priCols, priVals, id)
	if err != nil {
		if isUniqueViolation(err) {
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
		if err := pgUpsertJoin(ctx, tx, jd, cols, vals, id); err != nil {
			return nil, fmt.Errorf("%w: upsert %s: %v", store.ErrInternal, jd.Table, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("%w: commit: %v", store.ErrInternal, err)
	}
	return clone, nil
}

// ─── Patch ───────────────────────────────────────────────────────────────────

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

// ─── Delete ───────────────────────────────────────────────────────────────────

func (s *Store) Delete(ctx context.Context, resourceType string, id string) error {
	if s.isColumnMode(resourceType) {
		cfg, _ := s.resCfg(resourceType)
		return s.deleteColumnMode(ctx, resourceType, id, cfg)
	}
	return s.deleteJSONMode(ctx, resourceType, id)
}

func (s *Store) deleteJSONMode(ctx context.Context, resourceType string, id string) error {
	table := s.tableName(resourceType)
	tag, err := s.pool.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, quoteIdent(table)), id)
	if err != nil {
		return fmt.Errorf("%w: delete: %v", store.ErrInternal, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
	}
	return nil
}

func (s *Store) deleteColumnMode(ctx context.Context, resourceType string, id string, cfg ResourceTableConfig) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: begin tx: %v", store.ErrInternal, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	idColName := primaryIDCol(cfg)
	// Delete from joined tables first (in reverse declaration order).
	for i := len(cfg.Joins) - 1; i >= 0; i-- {
		jd := cfg.Joins[i]
		if !jd.DeleteJoin || jd.ForeignKey == "" {
			continue
		}
		q := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", quoteIdent(jd.Table), quoteIdent(jd.ForeignKey))
		if _, err := tx.Exec(ctx, q, id); err != nil {
			return fmt.Errorf("%w: delete %s: %v", store.ErrInternal, jd.Table, err)
		}
	}

	tag, err := tx.Exec(ctx,
		fmt.Sprintf("DELETE FROM %s WHERE %s = $1", quoteIdent(cfg.Table), quoteIdent(idColName)),
		id,
	)
	if err != nil {
		return fmt.Errorf("%w: delete: %v", store.ErrInternal, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%w: commit: %v", store.ErrInternal, err)
	}
	return nil
}

// ─── Column-mode helpers ──────────────────────────────────────────────────────

// fieldEntry is an ordered (SCIM path, qualified SQL column) pair used when
// building SELECT lists and scanning results for column-mode resources.
type fieldEntry struct {
	scimAttr string
	col      string // e.g. "\"accounts\".\"email\""
}

// buildColumnSelect returns the ordered field entries, the full SELECT…FROM…JOIN
// query (without WHERE), and the qualified id column expression.
func buildColumnSelect(cfg ResourceTableConfig) ([]fieldEntry, string, string) {
	entries := orderedEntries(cfg)
	cols := make([]string, len(entries))
	for i, e := range entries {
		cols[i] = e.col
	}
	from := quoteIdent(cfg.Table) + buildJoinSQL(cfg)
	q := "SELECT " + strings.Join(cols, ", ") + " FROM " + from

	idCol := quoteIdent(cfg.Table) + "." + quoteIdent(primaryIDCol(cfg))
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
			col:      quoteIdent(tbl) + "." + quoteIdent(ref.Column),
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
		fmt.Fprintf(&sb, " %s JOIN %s", jt, quoteIdent(jd.Table))
		if jd.Alias != "" {
			sb.WriteString(" " + quoteIdent(jd.Alias))
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

// splitByTable partitions FieldMappings into primary-table and per-joined-table groups.
// excludeCol (if non-empty) is removed from the primary-table group (used to exclude
// the id column from UPDATE SET clauses).
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

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJSONRow(row rowScanner) (*resource.Resource, error) {
	var data []byte
	if err := row.Scan(&data); err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal resource JSON: %w", err)
	}
	return resource.FromMap(m), nil
}

func scanColumnRow(row rowScanner, entries []fieldEntry, resourceType string) (*resource.Resource, error) {
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

func pgInsert(ctx context.Context, tx pgx.Tx, table string, cols []string, vals []interface{}) error {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteIdent(c)
	}
	phs := make([]string, len(vals))
	for i := range vals {
		phs[i] = fmt.Sprintf("$%d", i+1)
	}
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quoteIdent(table), strings.Join(quoted, ", "), strings.Join(phs, ", "),
	)
	_, err := tx.Exec(ctx, q, vals...)
	return err
}

func pgUpdate(ctx context.Context, tx pgx.Tx, table, idCol string, cols []string, vals []interface{}, id string) (bool, error) {
	if len(cols) == 0 {
		return true, nil // nothing to update but row exists (checked separately)
	}
	setClauses := make([]string, len(cols))
	for i, c := range cols {
		setClauses[i] = fmt.Sprintf("%s = $%d", quoteIdent(c), i+2)
	}
	q := fmt.Sprintf("UPDATE %s SET %s WHERE %s = $1",
		quoteIdent(table), strings.Join(setClauses, ", "), quoteIdent(idCol),
	)
	args := append([]interface{}{id}, vals...)
	tag, err := tx.Exec(ctx, q, args...)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// pgUpsertJoin inserts or updates a joined-table row using ON CONFLICT.
// Requires a unique/primary-key constraint on jd.ForeignKey in the joined table.
func pgUpsertJoin(ctx context.Context, tx pgx.Tx, jd JoinDef, cols []string, vals []interface{}, id string) error {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteIdent(c)
	}
	allCols := append([]string{quoteIdent(jd.ForeignKey)}, quoted...)
	allVals := append([]interface{}{id}, vals...)

	phs := make([]string, len(allVals))
	for i := range allVals {
		phs[i] = fmt.Sprintf("$%d", i+1)
	}
	setClauses := make([]string, len(quoted))
	for i, c := range quoted {
		setClauses[i] = fmt.Sprintf("%s = EXCLUDED.%s", c, c)
	}
	q := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s",
		quoteIdent(jd.Table),
		strings.Join(allCols, ", "),
		strings.Join(phs, ", "),
		quoteIdent(jd.ForeignKey),
		strings.Join(setClauses, ", "),
	)
	_, err := tx.Exec(ctx, q, allVals...)
	return err
}

func extractUserName(r *resource.Resource) string {
	for k, v := range r.Attributes {
		if strings.EqualFold(k, "userName") {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func extractActive(r *resource.Resource) bool {
	for k, v := range r.Attributes {
		if strings.EqualFold(k, "active") {
			if b, ok := v.(bool); ok {
				return b
			}
		}
	}
	return true
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(err.Error(), "unique constraint") ||
		strings.Contains(err.Error(), "unique_violation")
}
