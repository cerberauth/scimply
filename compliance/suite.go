package compliance

import (
	"net/http"
	"testing"
)

type SuiteConfig struct {
	BaseURL string

	AuthHeader string

	SCIMVersion string

	SkipBulk bool

	SkipETags bool

	HTTPClient *http.Client
}

func RunSuite(t *testing.T, cfg SuiteConfig) {
	t.Helper()

	if cfg.BaseURL == "" {
		t.Skip("compliance: BaseURL is not set")
	}
	if cfg.SCIMVersion == "" {
		cfg.SCIMVersion = "v2"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}

	t.Run("ServiceProviderConfig", func(t *testing.T) {
		testServiceProviderConfig(t, cfg)
	})
	t.Run("Schemas", func(t *testing.T) {
		testSchemas(t, cfg)
	})
	t.Run("ResourceTypes", func(t *testing.T) {
		testResourceTypes(t, cfg)
	})

	t.Run("Users/Create", func(t *testing.T) {
		testCreateUser(t, cfg)
	})
	t.Run("Users/Get", func(t *testing.T) {
		testGetUser(t, cfg)
	})
	t.Run("Users/List", func(t *testing.T) {
		testListUsers(t, cfg)
	})
	t.Run("Users/Filter", func(t *testing.T) {
		testFilterUsers(t, cfg)
	})
	t.Run("Users/Pagination", func(t *testing.T) {
		testPagination(t, cfg)
	})
	t.Run("Users/Replace", func(t *testing.T) {
		testReplaceUser(t, cfg)
	})
	t.Run("Users/Patch/Add", func(t *testing.T) {
		testPatchAdd(t, cfg)
	})
	t.Run("Users/Patch/Remove", func(t *testing.T) {
		testPatchRemove(t, cfg)
	})
	t.Run("Users/Patch/Replace", func(t *testing.T) {
		testPatchReplace(t, cfg)
	})
	t.Run("Users/Patch/Complex", func(t *testing.T) {
		testPatchComplex(t, cfg)
	})
	t.Run("Users/Delete", func(t *testing.T) {
		testDeleteUser(t, cfg)
	})

	t.Run("Groups/Create", func(t *testing.T) {
		testCreateGroup(t, cfg)
	})
	t.Run("Groups/MembershipPatch", func(t *testing.T) {
		testGroupMembership(t, cfg)
	})
	t.Run("Groups/Delete", func(t *testing.T) {
		testDeleteGroup(t, cfg)
	})

	t.Run("Errors/NotFound", func(t *testing.T) {
		testNotFound(t, cfg)
	})
	t.Run("Errors/Conflict", func(t *testing.T) {
		testConflict(t, cfg)
	})
	t.Run("Errors/BadFilter", func(t *testing.T) {
		testBadFilter(t, cfg)
	})
	t.Run("ContentType", func(t *testing.T) {
		testContentType(t, cfg)
	})
}
