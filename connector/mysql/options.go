package mysql

import (
	"strings"
	"time"

	sqlconn "github.com/cerberauth/scimply/connector/sql"
	"github.com/cerberauth/scimply/schema"
)

// JoinDef describes an additional table to JOIN when reading or writing a resource type.
type JoinDef struct {
	Table      string // table name to join
	Alias      string // optional SQL alias (defaults to Table)
	Condition  string // raw ON condition e.g. "profiles.user_id = accounts.id"
	JoinType   string // "LEFT" (default), "INNER", "RIGHT"
	ForeignKey string // column in this table referencing the primary table's id; required for writes
	DeleteJoin bool   // if true, DELETE from this table before deleting from the primary table
}

// ResourceTableConfig maps a SCIM resource type to a specific table and column layout.
// When FieldMappings is non-empty the connector switches to column mode for that resource
// type: reads and writes use the declared columns instead of a JSON data column, and
// AutoMigrate is skipped for that type.
type ResourceTableConfig struct {
	Table         string
	FieldMappings map[string]sqlconn.ColumnRef // SCIM attribute path -> column reference
	Joins         []JoinDef
}

type config struct {
	dsn             string
	tablePrefix     string
	autoMigrate     bool
	registry        *schema.Registry
	maxConns        int
	connTimeout     time.Duration
	resourceConfigs map[string]ResourceTableConfig // lowercase resource type -> config
}

func defaultConfig() config {
	return config{
		tablePrefix:     "scim_",
		maxConns:        10,
		connTimeout:     30 * time.Second,
		resourceConfigs: make(map[string]ResourceTableConfig),
	}
}

type Option func(*config)

func WithDSN(dsn string) Option {
	return func(c *config) { c.dsn = dsn }
}

func WithTablePrefix(prefix string) Option {
	return func(c *config) { c.tablePrefix = prefix }
}

func WithAutoMigrate(enabled bool) Option {
	return func(c *config) { c.autoMigrate = enabled }
}

func WithSchemaRegistry(reg *schema.Registry) Option {
	return func(c *config) { c.registry = reg }
}

func WithMaxConns(n int) Option {
	return func(c *config) { c.maxConns = n }
}

// WithTableName points a SCIM resource type at a specific table name.
// Without any FieldMappings the connector stays in JSON mode and the table
// must contain the standard scimply columns (id, data, …).
func WithTableName(resourceType, tableName string) Option {
	return func(c *config) {
		key := strings.ToLower(resourceType)
		cfg := c.resourceConfigs[key]
		cfg.Table = tableName
		c.resourceConfigs[key] = cfg
	}
}

// WithFieldMapping maps a SCIM attribute path to a column in the primary table.
// Setting any field mapping switches the resource type into column mode.
func WithFieldMapping(resourceType, scimAttr, column string) Option {
	return func(c *config) {
		key := strings.ToLower(resourceType)
		cfg := c.resourceConfigs[key]
		if cfg.FieldMappings == nil {
			cfg.FieldMappings = make(map[string]sqlconn.ColumnRef)
		}
		cfg.FieldMappings[scimAttr] = sqlconn.ColumnRef{Column: column}
		c.resourceConfigs[key] = cfg
	}
}

// WithTableFieldMapping maps a SCIM attribute path to a column in a specific joined table.
// The named table must also be registered with WithJoin so the SELECT includes the JOIN.
func WithTableFieldMapping(resourceType, scimAttr, table, column string) Option {
	return func(c *config) {
		key := strings.ToLower(resourceType)
		cfg := c.resourceConfigs[key]
		if cfg.FieldMappings == nil {
			cfg.FieldMappings = make(map[string]sqlconn.ColumnRef)
		}
		cfg.FieldMappings[scimAttr] = sqlconn.ColumnRef{Table: table, Column: column}
		c.resourceConfigs[key] = cfg
	}
}

// WithJoin adds a JOIN definition for a resource type.
// Required when any FieldMapping references a table other than the primary table.
func WithJoin(resourceType string, join JoinDef) Option {
	return func(c *config) {
		key := strings.ToLower(resourceType)
		cfg := c.resourceConfigs[key]
		cfg.Joins = append(cfg.Joins, join)
		c.resourceConfigs[key] = cfg
	}
}

// WithResourceConfig sets the complete table/field/join configuration for a resource type.
// This is the all-in-one alternative to combining WithTableName, WithFieldMapping, and WithJoin.
func WithResourceConfig(resourceType string, resCfg ResourceTableConfig) Option {
	return func(c *config) {
		c.resourceConfigs[strings.ToLower(resourceType)] = resCfg
	}
}
