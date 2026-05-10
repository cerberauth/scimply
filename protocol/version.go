package protocol

import "strings"

type Version int

const (
	V2_0 Version = iota
	V1_1
)

func (v Version) String() string {
	switch v {
	case V2_0:
		return "2.0"
	case V1_1:
		return "1.1"
	}
	return "2.0"
}

func DetectVersion(urlPath, contentType string) Version {
	lower := strings.ToLower(urlPath)
	if strings.HasPrefix(lower, "/v2/") || lower == "/v2" {
		return V2_0
	}
	if strings.HasPrefix(lower, "/v1/") || lower == "/v1" {
		return V1_1
	}
	if strings.Contains(strings.ToLower(contentType), ContentTypeSCIM) {
		return V2_0
	}
	return V2_0
}
