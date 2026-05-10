package sql

import (
	"strings"

	"github.com/cerberauth/scimply/schema"
)

type TableDef struct {
	MainTable string
	AuxTables []string
}

func GenerateTableDef(rt *schema.ResourceType, s *schema.Schema, tablePrefix string, dialect Dialect) TableDef {
	mainTableName := tablePrefix + strings.ToLower(rt.Name) + "s"

	var mainCols []string
	var auxTables []string

	mainCols = append(mainCols,
		idCol(dialect),
		textCol("external_id", false, dialect),
	)

	mainCols = append(mainCols,
		timestampCol("meta_created", dialect),
		timestampCol("meta_last_modified", dialect),
		textCol("meta_location", false, dialect),
		textCol("meta_version", false, dialect),
	)

	mapper := NewMapper()

	for _, attr := range s.Attributes {
		if attr.MultiValued {

			auxTable := generateAuxTable(mainTableName, attr, tablePrefix, dialect, mapper)
			if auxTable != "" {
				auxTables = append(auxTables, auxTable)
			}
			continue
		}

		if attr.Type == schema.TypeComplex {

			for _, sub := range attr.SubAttributes {
				if sub.MultiValued {

					auxTable := generateAuxTable(mainTableName, sub, tablePrefix, dialect, mapper)
					if auxTable != "" {
						auxTables = append(auxTables, auxTable)
					}
					continue
				}
				colPath := attr.Name + "." + sub.Name
				col, ok := mapper.ColumnName(colPath)
				if !ok {
					continue
				}
				mainCols = append(mainCols, scimAttrToColDef(col, sub, dialect))
			}
			continue
		}

		col, ok := mapper.ColumnName(attr.Name)
		if !ok {
			continue
		}
		mainCols = append(mainCols, scimAttrToColDef(col, attr, dialect))
	}

	mainTable := buildCreateTable(mainTableName, mainCols, dialect)

	return TableDef{
		MainTable: mainTable,
		AuxTables: auxTables,
	}
}

func generateAuxTable(mainTableName string, attr schema.Attribute, tablePrefix string, dialect Dialect, mapper *Mapper) string {
	auxTableName := mainTableName + "_" + strings.ToLower(attr.Name)

	var cols []string
	cols = append(cols, idCol(dialect))

	cols = append(cols, fkCol("resource_id", mainTableName, dialect))

	if attr.Type == schema.TypeComplex {
		for _, sub := range attr.SubAttributes {
			col, ok := mapper.ColumnName(sub.Name)
			if !ok {
				continue
			}
			cols = append(cols, scimAttrToColDef(col, sub, dialect))
		}
	} else {

		cols = append(cols, scimAttrToColDef("value", attr, dialect))
	}

	return buildCreateTable(auxTableName, cols, dialect)
}

func scimAttrToColDef(colName string, attr schema.Attribute, dialect Dialect) string {
	switch attr.Type {
	case schema.TypeBoolean:
		return boolCol(colName, attr.Required, dialect)
	case schema.TypeInteger:
		return intCol(colName, attr.Required, dialect)
	case schema.TypeDecimal:
		return decimalCol(colName, attr.Required, dialect)
	case schema.TypeDateTime:
		return timestampCol(colName, dialect)
	case schema.TypeBinary:
		return blobCol(colName, dialect)
	default:

		return textCol(colName, attr.Required, dialect)
	}
}

func buildCreateTable(tableName string, cols []string, _ Dialect) string {
	var sb strings.Builder
	sb.WriteString("CREATE TABLE IF NOT EXISTS ")
	sb.WriteString(tableName)
	sb.WriteString(" (\n")
	for i, col := range cols {
		sb.WriteString("    ")
		sb.WriteString(col)
		if i < len(cols)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(")")
	return sb.String()
}

func idCol(dialect Dialect) string {
	switch dialect {
	case DialectMySQL:
		return "id VARCHAR(255) NOT NULL PRIMARY KEY"
	default:
		return "id TEXT NOT NULL PRIMARY KEY"
	}
}

func textCol(name string, required bool, dialect Dialect) string {
	null := nullConstraint(required)
	switch dialect {
	case DialectMySQL:
		return name + " TEXT" + null
	default:
		return name + " TEXT" + null
	}
}

func boolCol(name string, required bool, dialect Dialect) string {
	null := nullConstraint(required)
	switch dialect {
	case DialectMySQL:
		return name + " TINYINT(1)" + null
	default:
		return name + " BOOLEAN" + null
	}
}

func intCol(name string, required bool, dialect Dialect) string {
	null := nullConstraint(required)
	return name + " INTEGER" + null
}

func decimalCol(name string, required bool, dialect Dialect) string {
	null := nullConstraint(required)
	return name + " NUMERIC" + null
}

func timestampCol(name string, dialect Dialect) string {
	switch dialect {
	case DialectMySQL:
		return name + " DATETIME NULL"
	default:
		return name + " TIMESTAMPTZ NULL"
	}
}

func blobCol(name string, dialect Dialect) string {
	switch dialect {
	case DialectMySQL:
		return name + " LONGBLOB NULL"
	default:
		return name + " BYTEA NULL"
	}
}

func fkCol(name, refTable string, dialect Dialect) string {
	switch dialect {
	case DialectMySQL:
		return name + " VARCHAR(255) NOT NULL REFERENCES " + refTable + "(id) ON DELETE CASCADE"
	default:
		return name + " TEXT NOT NULL REFERENCES " + refTable + "(id) ON DELETE CASCADE"
	}
}

func nullConstraint(required bool) string {
	if required {
		return " NOT NULL"
	}
	return " NULL"
}
