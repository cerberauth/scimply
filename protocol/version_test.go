package protocol

import "testing"

func TestVersionString(t *testing.T) {
	if got := V2_0.String(); got != "2.0" {
		t.Errorf("V2_0.String() = %q, want %q", got, "2.0")
	}
	if got := V1_1.String(); got != "1.1" {
		t.Errorf("V1_1.String() = %q, want %q", got, "1.1")
	}
}

func TestDetectVersion(t *testing.T) {
	tests := []struct {
		urlPath     string
		contentType string
		want        Version
	}{
		{"/v2/Users", "", V2_0},
		{"/v2/", "", V2_0},
		{"/v2", "", V2_0},
		{"/v1/Users", "", V1_1},
		{"/v1/", "", V1_1},
		{"/v1", "", V1_1},
		{"/Users", "application/scim+json", V2_0},
		{"/Users", "application/json", V2_0},
		{"/Users", "", V2_0},
		{"", "", V2_0},
		{"/V2/Users", "", V2_0},
		{"/V1/Groups", "", V1_1},
	}
	for _, tc := range tests {
		got := DetectVersion(tc.urlPath, tc.contentType)
		if got != tc.want {
			t.Errorf("DetectVersion(%q, %q) = %v, want %v", tc.urlPath, tc.contentType, got, tc.want)
		}
	}
}
