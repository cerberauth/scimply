package audit

import "context"

type NoopLogger struct{}

func (n *NoopLogger) Log(_ context.Context, _ Event) error {
	return nil
}

func Noop() Logger {
	return &NoopLogger{}
}
