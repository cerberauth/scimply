package postgres

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	sqlconn "github.com/cerberauth/scimply/connector/sql"
	"github.com/cerberauth/scimply/resource"
	"github.com/cerberauth/scimply/store"
)

func (s *Store) buildListQuery(resourceType string, params store.ListParams) (string, []interface{}, error) {
	table := s.tableName(resourceType)

	var whereClauses []string
	var args []interface{}
	argIdx := 1

	if params.Filter != nil {
		result := sqlconn.TranslateFilter(params.Filter, s.mapper, sqlconn.DialectPostgres, "")
		if result.Err != nil {
			return "", nil, result.Err
		}
		if result.Clause != "" {
			whereClauses = append(whereClauses, result.Clause)
			args = append(args, result.Args...)
			argIdx += len(result.Args)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, `SELECT data FROM %s`, quoteIdent(table))

	if len(whereClauses) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(whereClauses, " AND "))
	}

	if params.SortBy != "" {
		col, ok := s.mapper.ColumnName(params.SortBy)
		if ok {
			dir := "ASC"
			if params.SortOrder == store.SortDescending {
				dir = "DESC"
			}
			fmt.Fprintf(&sb, " ORDER BY %s %s", quoteIdent(col), dir)
		}
	}

	if params.Count > 0 {
		fmt.Fprintf(&sb, " LIMIT $%d", argIdx)
		args = append(args, params.Count)
		argIdx++
	}
	if params.StartIndex > 1 {
		offset := params.StartIndex - 1
		fmt.Fprintf(&sb, " OFFSET $%d", argIdx)
		args = append(args, offset)
	}

	return sb.String(), args, nil
}

func (s *Store) buildCountQuery(resourceType string, filter resource.FilterExpression) (string, []interface{}, error) {
	table := s.tableName(resourceType)

	if filter == nil {
		q := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, quoteIdent(table))
		return q, nil, nil
	}

	result := sqlconn.TranslateFilter(filter, s.mapper, sqlconn.DialectPostgres, "")
	if result.Err != nil {
		return "", nil, result.Err
	}

	var q string
	if result.Clause != "" {
		q = fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s`, quoteIdent(table), result.Clause)
	} else {
		q = fmt.Sprintf(`SELECT COUNT(*) FROM %s`, quoteIdent(table))
	}
	return q, result.Args, nil
}

func (s *Store) buildFallbackQuery(resourceType string) (string, []interface{}, error) {
	table := s.tableName(resourceType)
	q := fmt.Sprintf(`SELECT data FROM %s LIMIT 1000`, quoteIdent(table))
	return q, nil, nil
}

func generateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	), nil
}
