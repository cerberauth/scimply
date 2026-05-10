package audit

import (
	"context"
	"encoding/json"
	"io"
	"sync"
)

type JSONLogger struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func NewJSONLogger(w io.Writer) *JSONLogger {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &JSONLogger{enc: enc}
}

func (l *JSONLogger) Log(_ context.Context, event Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.enc.Encode(event)
}
