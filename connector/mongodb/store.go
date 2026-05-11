package mongodb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/cerberauth/scimply/resource"
	"github.com/cerberauth/scimply/store"
)

type Store struct {
	cfg    config
	client *mongo.Client
	db     *mongo.Database
}

// Database returns the underlying MongoDB database for custom queries.
func (s *Store) Database() *mongo.Database { return s.db }

func New(opts ...Option) (*Store, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.uri == "" {
		return nil, fmt.Errorf("mongodb store: URI is required (use WithURI)")
	}
	if cfg.database == "" {
		return nil, fmt.Errorf("mongodb store: database name is required (use WithDatabase)")
	}
	return &Store{cfg: cfg}, nil
}

func (s *Store) Init(ctx context.Context) error {
	clientOpts := options.Client().ApplyURI(s.cfg.uri)
	if s.cfg.timeout > 0 {
		clientOpts.SetConnectTimeout(s.cfg.timeout)
		clientOpts.SetServerSelectionTimeout(s.cfg.timeout)
	}

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return fmt.Errorf("mongodb store: connect: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return fmt.Errorf("mongodb store: ping: %w", err)
	}

	s.client = client
	s.db = client.Database(s.cfg.database)

	if s.cfg.autoMigrate {
		if err := s.ensureIndexes(ctx); err != nil {
			return fmt.Errorf("mongodb store: indexes: %w", err)
		}
	}
	return nil
}

const userNameAttr = "userName"

func (s *Store) ensureIndexes(ctx context.Context) error {
	// Skip index creation for resource types that have explicit field mappings.
	userCfg := s.cfg.resourceConfigs["user"]
	if len(userCfg.FieldMappings) == 0 {
		usersColl := s.db.Collection(s.collectionName("user"))
		userNameIdx := mongo.IndexModel{
			Keys:    bson.D{{Key: userNameAttr, Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true).SetName("unique_userName"),
		}
		if _, err := usersColl.Indexes().CreateOne(ctx, userNameIdx); err != nil {
			return fmt.Errorf("create userName index: %w", err)
		}
	}
	return nil
}

func (s *Store) Close(ctx context.Context) error {
	if s.client != nil {
		return s.client.Disconnect(ctx)
	}
	return nil
}

func (s *Store) Healthy(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("mongodb store: not initialized")
	}
	return s.client.Ping(ctx, nil)
}

func (s *Store) collectionName(resourceType string) string {
	key := strings.ToLower(resourceType)
	if cfg, ok := s.cfg.resourceConfigs[key]; ok && cfg.Collection != "" {
		return cfg.Collection
	}
	return s.cfg.collPrefix + strings.ToLower(resourceType) + "s"
}

func (s *Store) isFieldMapped(resourceType string) bool {
	cfg, ok := s.cfg.resourceConfigs[strings.ToLower(resourceType)]
	return ok && len(cfg.FieldMappings) > 0
}

func (s *Store) resCfg(resourceType string) (ResourceCollectionConfig, bool) {
	cfg, ok := s.cfg.resourceConfigs[strings.ToLower(resourceType)]
	return cfg, ok
}

func (s *Store) Create(ctx context.Context, resourceType string, res *resource.Resource) (*resource.Resource, error) {
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

	var doc bson.M
	if s.isFieldMapped(resourceType) {
		cfg, _ := s.resCfg(resourceType)
		doc = resourceToMappedDoc(clone, cfg)
	} else {
		doc = resourceToDoc(clone)
	}

	coll := s.db.Collection(s.collectionName(resourceType))
	if _, err := coll.InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("%w: %v", store.ErrConflict, err)
		}
		return nil, fmt.Errorf("%w: insert: %v", store.ErrInternal, err)
	}
	return clone, nil
}

func (s *Store) Get(ctx context.Context, resourceType string, id string) (*resource.Resource, error) {
	if cfg, ok := s.resCfg(resourceType); ok && (len(cfg.FieldMappings) > 0 || len(cfg.Lookups) > 0) {
		return s.getWithConfig(ctx, resourceType, id, cfg)
	}
	return s.getSimple(ctx, resourceType, id)
}

func (s *Store) getSimple(ctx context.Context, resourceType string, id string) (*resource.Resource, error) {
	coll := s.db.Collection(s.collectionName(resourceType))
	var doc bson.M
	if err := coll.FindOne(ctx, bson.D{{Key: mongoID, Value: id}}).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
		}
		return nil, fmt.Errorf("%w: findOne: %v", store.ErrInternal, err)
	}
	return docToResource(doc, resourceType), nil
}

func (s *Store) getWithConfig(ctx context.Context, resourceType string, id string, cfg ResourceCollectionConfig) (*resource.Resource, error) {
	coll := s.db.Collection(s.collectionName(resourceType))

	if len(cfg.Lookups) == 0 {
		var doc bson.M
		if err := coll.FindOne(ctx, bson.D{{Key: mongoID, Value: id}}).Decode(&doc); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
			}
			return nil, fmt.Errorf("%w: findOne: %v", store.ErrInternal, err)
		}
		return mappedDocToResource(doc, cfg, resourceType), nil
	}

	// Use aggregation pipeline when lookups are configured.
	pipeline := buildAggregatePipeline(id, cfg)
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("%w: aggregate: %v", store.ErrInternal, err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	if !cursor.Next(ctx) {
		if err := cursor.Err(); err != nil {
			return nil, fmt.Errorf("%w: cursor: %v", store.ErrInternal, err)
		}
		return nil, fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
	}
	var doc bson.M
	if err := cursor.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", store.ErrInternal, err)
	}
	return mappedDocToResource(doc, cfg, resourceType), nil
}

func (s *Store) List(ctx context.Context, resourceType string, params store.ListParams) (*store.ListResult, error) {
	coll := s.db.Collection(s.collectionName(resourceType))

	filter := bson.D{}
	needsClientFilter := false
	if params.Filter != nil {
		bsonFilter, err := TranslateFilter(params.Filter)
		if err != nil {
			needsClientFilter = true
		} else {
			filter = bsonFilter
		}
	}

	total64, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("%w: count: %v", store.ErrInternal, err)
	}
	total := int(total64)

	startIndex := params.StartIndex
	if startIndex < 1 {
		startIndex = 1
	}
	offset := int64(startIndex - 1)
	limit := int64(params.Count)
	if limit <= 0 {
		limit = int64(total)
	}

	findOpts := options.Find().SetSkip(offset)
	if limit > 0 {
		findOpts.SetLimit(limit)
	}
	if params.SortBy != "" {
		dir := 1
		if params.SortOrder == store.SortDescending {
			dir = -1
		}
		// Respect field mapping for sort field if configured.
		sortField := params.SortBy
		if cfg, ok := s.resCfg(resourceType); ok {
			if mapped, ok := cfg.FieldMappings[params.SortBy]; ok {
				sortField = mapped
			}
		}
		findOpts.SetSort(bson.D{{Key: sortField, Value: dir}})
	}

	cursor, err := coll.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, fmt.Errorf("%w: find: %v", store.ErrInternal, err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	cfg, hasCfg := s.resCfg(resourceType)

	var resources []*resource.Resource
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("%w: decode: %v", store.ErrInternal, err)
		}
		var r *resource.Resource
		if hasCfg && len(cfg.FieldMappings) > 0 {
			r = mappedDocToResource(doc, cfg, resourceType)
		} else {
			r = docToResource(doc, resourceType)
		}
		resources = append(resources, r)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("%w: cursor: %v", store.ErrInternal, err)
	}

	return &store.ListResult{
		Resources:         resources,
		TotalResults:      total,
		StartIndex:        startIndex,
		ItemsPerPage:      len(resources),
		NeedsClientFilter: needsClientFilter,
	}, nil
}

func (s *Store) Replace(ctx context.Context, resourceType string, id string, res *resource.Resource) (*resource.Resource, error) {
	existing, err := s.Get(ctx, resourceType, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	clone := res.Clone()
	clone.ID = id
	clone.Meta.Created = existing.Meta.Created
	clone.Meta.ResourceType = resourceType
	clone.Meta.LastModified = now

	var doc bson.M
	if s.isFieldMapped(resourceType) {
		cfg, _ := s.resCfg(resourceType)
		doc = resourceToMappedDoc(clone, cfg)
	} else {
		doc = resourceToDoc(clone)
	}

	coll := s.db.Collection(s.collectionName(resourceType))
	result, err := coll.ReplaceOne(ctx, bson.D{{Key: mongoID, Value: id}}, doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("%w: %v", store.ErrConflict, err)
		}
		return nil, fmt.Errorf("%w: replace: %v", store.ErrInternal, err)
	}
	if result.MatchedCount == 0 {
		return nil, fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
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
	coll := s.db.Collection(s.collectionName(resourceType))
	result, err := coll.DeleteOne(ctx, bson.D{{Key: mongoID, Value: id}})
	if err != nil {
		return fmt.Errorf("%w: delete: %v", store.ErrInternal, err)
	}
	if result.DeletedCount == 0 {
		return fmt.Errorf("%w: %s/%s", store.ErrNotFound, resourceType, id)
	}
	return nil
}

// resourceToDoc converts a Resource to a BSON document using SCIM attribute names as keys.
func resourceToDoc(r *resource.Resource) bson.M {
	m := r.ToMap()
	doc := bson.M{}
	for k, v := range m {
		if strings.EqualFold(k, "id") {
			doc[mongoID] = v
		} else {
			doc[k] = v
		}
	}
	return doc
}

// docToResource converts a BSON document (with SCIM attribute name keys) back to a Resource.
func docToResource(doc bson.M, resourceType string) *resource.Resource {
	m := make(map[string]interface{}, len(doc))
	for k, v := range doc {
		if k == mongoID {
			m["id"] = v
		} else {
			m[k] = v
		}
	}
	r := resource.FromMap(m)
	r.Meta.ResourceType = resourceType
	return r
}

// resourceToMappedDoc converts a Resource to a BSON document using the configured field mappings.
// Unmapped attributes are stored under their SCIM attribute names.
func resourceToMappedDoc(r *resource.Resource, cfg ResourceCollectionConfig) bson.M {
	// Build a flat attribute map then apply field mappings.
	flatMap := flattenResource(r)
	doc := bson.M{}

	// Apply explicit mappings.
	mapped := make(map[string]bool)
	for scimAttr, bsonPath := range cfg.FieldMappings {
		val, ok := flatMap[scimAttr]
		if !ok {
			continue
		}
		if scimAttr == "id" {
			doc[mongoID] = val
		} else {
			setDotPath(doc, bsonPath, val)
		}
		mapped[scimAttr] = true
	}

	// Store unmapped attributes under their SCIM names.
	for k, v := range flatMap {
		if mapped[k] {
			continue
		}
		if k == "id" {
			if _, hasID := doc[mongoID]; !hasID {
				doc[mongoID] = v
			}
			continue
		}
		setDotPath(doc, k, v)
	}
	return doc
}

// mappedDocToResource converts a BSON document back to a Resource using field mappings.
func mappedDocToResource(doc bson.M, cfg ResourceCollectionConfig, resourceType string) *resource.Resource {
	// Build reverse mapping: bson path -> scim attr
	reverseMap := make(map[string]string, len(cfg.FieldMappings))
	for scimAttr, bsonPath := range cfg.FieldMappings {
		reverseMap[bsonPath] = scimAttr
	}

	// Flatten the document into dot-notation paths.
	flat := flattenDoc(doc, "")

	attrMap := make(map[string]interface{})
	used := make(map[string]bool)

	// Apply reverse mappings.
	for bsonPath, scimAttr := range reverseMap {
		if val, ok := flat[bsonPath]; ok {
			attrMap[scimAttr] = val
			used[bsonPath] = true
		}
	}

	// _id -> id
	if _, hasID := attrMap["id"]; !hasID {
		if v, ok := flat[mongoID]; ok {
			attrMap["id"] = v
			used[mongoID] = true
		}
	}

	// Pass through unmapped fields.
	for path, val := range flat {
		if used[path] {
			continue
		}
		attrMap[path] = val
	}

	r := &resource.Resource{Attributes: make(map[string]interface{})}
	r.Meta.ResourceType = resourceType
	for path, val := range attrMap {
		applyAttrToResource(r, path, val)
	}
	return r
}

func applyAttrToResource(r *resource.Resource, path string, val interface{}) {
	switch strings.ToLower(path) {
	case "id":
		if s, ok := val.(string); ok {
			r.ID = s
		}
	case "externalid":
		if s, ok := val.(string); ok {
			r.ExternalID = s
		}
	case "meta.created":
		if t, ok := val.(time.Time); ok {
			r.Meta.Created = t
		}
	case "meta.lastmodified":
		if t, ok := val.(time.Time); ok {
			r.Meta.LastModified = t
		}
	case "meta.version":
		if s, ok := val.(string); ok {
			r.Meta.Version = s
		}
	default:
		setNestedAttr(r.Attributes, path, val)
	}
}

func setNestedAttr(attrs map[string]interface{}, path string, val interface{}) {
	dot := strings.IndexByte(path, '.')
	if dot < 0 {
		attrs[path] = val
		return
	}
	parent, rest := path[:dot], path[dot+1:]
	sub, ok := attrs[parent].(map[string]interface{})
	if !ok {
		sub = make(map[string]interface{})
	}
	setNestedAttr(sub, rest, val)
	attrs[parent] = sub
}

// flattenResource returns a flat map of dot-notation paths to values from a Resource.
func flattenResource(r *resource.Resource) map[string]interface{} {
	m := map[string]interface{}{
		"id":                r.ID,
		"externalId":        r.ExternalID,
		"meta.created":      r.Meta.Created,
		"meta.lastModified": r.Meta.LastModified,
		"meta.version":      r.Meta.Version,
	}
	flattenMap(r.Attributes, "", m)
	return m
}

func flattenMap(attrs map[string]interface{}, prefix string, out map[string]interface{}) {
	for k, v := range attrs {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v.(map[string]interface{}); ok {
			flattenMap(sub, key, out)
		} else {
			out[key] = v
		}
	}
}

// flattenDoc flattens a BSON document into dot-notation paths.
func flattenDoc(doc bson.M, prefix string) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range doc {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v.(bson.M); ok {
			for subK, subV := range flattenDoc(sub, key) {
				out[subK] = subV
			}
		} else {
			out[key] = v
		}
	}
	return out
}

// setDotPath writes a value into a nested bson.M using dot-notation path.
func setDotPath(doc bson.M, path string, val interface{}) {
	dot := strings.IndexByte(path, '.')
	if dot < 0 {
		doc[path] = val
		return
	}
	parent, rest := path[:dot], path[dot+1:]
	sub, ok := doc[parent].(bson.M)
	if !ok {
		sub = bson.M{}
	}
	setDotPath(sub, rest, val)
	doc[parent] = sub
}

// buildAggregatePipeline builds a $match + $lookup aggregation pipeline for a single document.
func buildAggregatePipeline(id string, cfg ResourceCollectionConfig) bson.A {
	pipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: mongoID, Value: id}}}},
	}
	for _, lk := range cfg.Lookups {
		pipeline = append(pipeline, bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: lk.From},
			{Key: "localField", Value: lk.LocalField},
			{Key: "foreignField", Value: lk.ForeignField},
			{Key: "as", Value: lk.As},
		}}})
		// Unwind to get a single object instead of an array (for 1:1 relationships).
		pipeline = append(pipeline, bson.D{{Key: "$unwind", Value: bson.D{
			{Key: "path", Value: "$" + lk.As},
			{Key: "preserveNullAndEmptyArrays", Value: true},
		}}})
	}
	return pipeline
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("%w: generate id: %v", store.ErrInternal, err)
	}
	return hex.EncodeToString(b), nil
}
