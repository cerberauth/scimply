package mongodb

import (
	"strings"
	"time"

	"github.com/cerberauth/scimply/schema"
)

// LookupConfig describes a cross-collection join via MongoDB's $lookup aggregation stage.
type LookupConfig struct {
	From          string            // source collection name
	LocalField    string            // field in the current document
	ForeignField  string            // field in the source collection
	As            string            // output field name in the aggregation result
	FieldMappings map[string]string // SCIM attr path -> "As.fieldname" path in the joined doc
}

// ResourceCollectionConfig maps a SCIM resource type to a specific collection and field layout.
// When FieldMappings is non-empty the connector reads and writes individual document fields
// using the declared paths instead of SCIM attribute names. AutoMigrate (index creation) is
// skipped for that type when FieldMappings is set.
type ResourceCollectionConfig struct {
	Collection    string
	FieldMappings map[string]string // SCIM attribute path -> BSON field path
	Lookups       []LookupConfig
}

type config struct {
	uri             string
	database        string
	collPrefix      string
	autoMigrate     bool
	registry        *schema.Registry
	timeout         time.Duration
	resourceConfigs map[string]ResourceCollectionConfig // lowercase resource type -> config
}

func defaultConfig() config {
	return config{
		collPrefix:      "scim_",
		timeout:         30 * time.Second,
		resourceConfigs: make(map[string]ResourceCollectionConfig),
	}
}

type Option func(*config)

func WithURI(uri string) Option {
	return func(c *config) { c.uri = uri }
}

func WithDatabase(db string) Option {
	return func(c *config) { c.database = db }
}

func WithCollectionPrefix(prefix string) Option {
	return func(c *config) { c.collPrefix = prefix }
}

func WithAutoMigrate(enabled bool) Option {
	return func(c *config) { c.autoMigrate = enabled }
}

func WithSchemaRegistry(reg *schema.Registry) Option {
	return func(c *config) { c.registry = reg }
}

func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithCollectionName points a SCIM resource type at a specific collection name.
func WithCollectionName(resourceType, collectionName string) Option {
	return func(c *config) {
		key := strings.ToLower(resourceType)
		cfg := c.resourceConfigs[key]
		cfg.Collection = collectionName
		c.resourceConfigs[key] = cfg
	}
}

// WithTableName is an alias for WithCollectionName for API consistency with SQL connectors.
func WithTableName(resourceType, collectionName string) Option {
	return WithCollectionName(resourceType, collectionName)
}

// WithFieldMapping maps a SCIM attribute path to a BSON field path within the document.
func WithFieldMapping(resourceType, scimAttr, bsonPath string) Option {
	return func(c *config) {
		key := strings.ToLower(resourceType)
		cfg := c.resourceConfigs[key]
		if cfg.FieldMappings == nil {
			cfg.FieldMappings = make(map[string]string)
		}
		cfg.FieldMappings[scimAttr] = bsonPath
		c.resourceConfigs[key] = cfg
	}
}

// WithLookup adds a cross-collection lookup for a resource type.
func WithLookup(resourceType string, lookup LookupConfig) Option {
	return func(c *config) {
		key := strings.ToLower(resourceType)
		cfg := c.resourceConfigs[key]
		cfg.Lookups = append(cfg.Lookups, lookup)
		c.resourceConfigs[key] = cfg
	}
}

// WithResourceConfig sets the complete collection/field/lookup configuration for a resource type.
func WithResourceConfig(resourceType string, resCfg ResourceCollectionConfig) Option {
	return func(c *config) {
		c.resourceConfigs[strings.ToLower(resourceType)] = resCfg
	}
}
