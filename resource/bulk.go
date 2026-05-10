package resource

import (
	"fmt"
	"strings"
)

type BulkMethod string

const (
	BulkMethodPost   BulkMethod = "POST"
	BulkMethodPut    BulkMethod = "PUT"
	BulkMethodPatch  BulkMethod = "PATCH"
	BulkMethodDelete BulkMethod = "DELETE"
)

type BulkOperation struct {
	Method  BulkMethod
	BulkID  string
	Path    string
	Data    interface{}
	Version string
}

type BulkRequest struct {
	Schemas      []string
	FailOnErrors int
	Operations   []BulkOperation
}

type BulkResponseOperation struct {
	Method   BulkMethod
	BulkID   string
	Location string
	Version  string
	Status   int
	Response interface{}
}

type BulkResponse struct {
	Schemas    []string
	Operations []BulkResponseOperation
}

func ParseBulkRequest(body map[string]interface{}) (*BulkRequest, error) {
	req := &BulkRequest{}

	if s := lookupBulkField(body, schemasKey); s != nil {
		switch sv := s.(type) {
		case []interface{}:
			for _, v := range sv {
				if str, ok := v.(string); ok {
					req.Schemas = append(req.Schemas, str)
				}
			}
		case []string:
			req.Schemas = sv
		}
	}

	if foe := lookupBulkField(body, "failOnErrors"); foe != nil {
		switch n := foe.(type) {
		case float64:
			req.FailOnErrors = int(n)
		case int:
			req.FailOnErrors = n
		case int64:
			req.FailOnErrors = int(n)
		}
	}

	opsRaw := lookupBulkField(body, "Operations")
	if opsRaw == nil {
		return nil, fmt.Errorf("%w: missing Operations field", ErrBadPatch)
	}

	ops, ok := opsRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%w: Operations must be an array", ErrBadPatch)
	}

	for i, opRaw := range ops {
		opMap, ok := opRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%w: bulk operation %d is not an object", ErrBadPatch, i)
		}
		op, err := parseBulkOp(opMap)
		if err != nil {
			return nil, fmt.Errorf("%w: bulk operation %d: %v", ErrBadPatch, i, err)
		}
		req.Operations = append(req.Operations, op)
	}

	return req, nil
}

func parseBulkOp(m map[string]interface{}) (BulkOperation, error) {
	op := BulkOperation{}

	methodStr, ok := lookupMapString(m, "method")
	if !ok {
		return op, fmt.Errorf("missing 'method' field")
	}
	switch strings.ToUpper(methodStr) {
	case "POST":
		op.Method = BulkMethodPost
	case "PUT":
		op.Method = BulkMethodPut
	case "PATCH":
		op.Method = BulkMethodPatch
	case "DELETE":
		op.Method = BulkMethodDelete
	default:
		return op, fmt.Errorf("unsupported method: %q", methodStr)
	}

	if bulkID, ok := lookupMapString(m, "bulkId"); ok {
		op.BulkID = bulkID
	}

	if path, ok := lookupMapString(m, "path"); ok {
		op.Path = path
	}

	if version, ok := lookupMapString(m, "version"); ok {
		op.Version = version
	}

	if data, ok := m["data"]; ok {
		op.Data = data
	}

	return op, nil
}

func lookupBulkField(m map[string]interface{}, key string) interface{} {
	lower := strings.ToLower(key)
	if v, ok := m[key]; ok {
		return v
	}
	for k, v := range m {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	return nil
}
