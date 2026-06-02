package store

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/cerberauth/scimply/resource"
)

const tracerName = "github.com/cerberauth/scimply/store"

type TracingStore struct {
	wrapped ResourceStore
	tracer  trace.Tracer
}

func NewTracingStore(s ResourceStore) *TracingStore {
	return &TracingStore{wrapped: s, tracer: otel.Tracer(tracerName)}
}

func (t *TracingStore) Create(ctx context.Context, resourceType string, res *resource.Resource) (*resource.Resource, error) {
	ctx, span := t.tracer.Start(ctx, "store.Create",
		trace.WithAttributes(attribute.String("scim.resource_type", resourceType)),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()
	result, err := t.wrapped.Create(ctx, resourceType, res)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

func (t *TracingStore) Get(ctx context.Context, resourceType string, id string) (*resource.Resource, error) {
	ctx, span := t.tracer.Start(ctx, "store.Get",
		trace.WithAttributes(
			attribute.String("scim.resource_type", resourceType),
			attribute.String("scim.resource_id", id),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()
	result, err := t.wrapped.Get(ctx, resourceType, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

func (t *TracingStore) List(ctx context.Context, resourceType string, params ListParams) (*ListResult, error) {
	ctx, span := t.tracer.Start(ctx, "store.List",
		trace.WithAttributes(attribute.String("scim.resource_type", resourceType)),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()
	result, err := t.wrapped.List(ctx, resourceType, params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

func (t *TracingStore) Replace(ctx context.Context, resourceType string, id string, res *resource.Resource) (*resource.Resource, error) {
	ctx, span := t.tracer.Start(ctx, "store.Replace",
		trace.WithAttributes(
			attribute.String("scim.resource_type", resourceType),
			attribute.String("scim.resource_id", id),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()
	result, err := t.wrapped.Replace(ctx, resourceType, id, res)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

func (t *TracingStore) Patch(ctx context.Context, resourceType string, id string, ops []resource.PatchOp) (*resource.Resource, error) {
	ctx, span := t.tracer.Start(ctx, "store.Patch",
		trace.WithAttributes(
			attribute.String("scim.resource_type", resourceType),
			attribute.String("scim.resource_id", id),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()
	result, err := t.wrapped.Patch(ctx, resourceType, id, ops)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

func (t *TracingStore) Delete(ctx context.Context, resourceType string, id string) error {
	ctx, span := t.tracer.Start(ctx, "store.Delete",
		trace.WithAttributes(
			attribute.String("scim.resource_type", resourceType),
			attribute.String("scim.resource_id", id),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()
	err := t.wrapped.Delete(ctx, resourceType, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}
