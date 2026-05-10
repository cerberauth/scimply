package compliance

func NewTestUser(suffix string) map[string]interface{} {
	return map[string]interface{}{
		"schemas":  []interface{}{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"userName": "test-user-" + suffix,
		"name": map[string]interface{}{
			"givenName":  "Test",
			"familyName": "User" + suffix,
		},
		"emails": []interface{}{
			map[string]interface{}{
				"value":   "test-" + suffix + "@example.com",
				"type":    "work",
				"primary": true,
			},
		},
		"active": true,
	}
}

func NewTestGroup(suffix string) map[string]interface{} {
	return map[string]interface{}{
		"schemas":     []interface{}{"urn:ietf:params:scim:schemas:core:2.0:Group"},
		"displayName": "Test Group " + suffix,
	}
}
