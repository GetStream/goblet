// Copyright 2026 GetStream, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package goblet

import (
	"context"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "github.com/GetStream/goblet"

// Attribute keys for goblet metrics.
const (
	// CommandTypeKey indicates a command type ("ls-refs", "fetch", "not-a-command").
	CommandTypeKey = attribute.Key("goblet.command_type")
	// CommandCacheStateKey indicates whether the command response is cached
	// or not ("locally-served", "queried-upstream").
	CommandCacheStateKey = attribute.Key("goblet.command_cache_state")
	// CommandCanonicalStatusKey indicates whether the command is succeeded
	// or not ("OK", "Unauthenticated").
	CommandCanonicalStatusKey = attribute.Key("goblet.command_status")
)

var (
	// InboundCommandCount is a count of inbound commands.
	InboundCommandCount metric.Int64Counter
	// OutboundCommandCount is a count of outbound commands.
	OutboundCommandCount metric.Int64Counter
	// InboundCommandProcessingTime is the processing time of inbound commands (ms).
	InboundCommandProcessingTime metric.Int64Histogram
	// OutboundCommandProcessingTime is the processing time of outbound commands (ms).
	OutboundCommandProcessingTime metric.Int64Histogram
	// UpstreamFetchWaitingTime is the time a fetch request waited for the upstream (ms).
	UpstreamFetchWaitingTime metric.Int64Histogram
)

func init() {
	meter := otel.Meter(meterName)

	var err error
	if InboundCommandCount, err = meter.Int64Counter(
		"goblet.inbound_command_count",
		metric.WithDescription("number of inbound commands"),
	); err != nil {
		log.Fatalf("metrics: %v", err)
	}
	if OutboundCommandCount, err = meter.Int64Counter(
		"goblet.outbound_command_count",
		metric.WithDescription("number of outbound commands"),
	); err != nil {
		log.Fatalf("metrics: %v", err)
	}
	if InboundCommandProcessingTime, err = meter.Int64Histogram(
		"goblet.inbound_command_processing_time",
		metric.WithDescription("processing time of inbound commands"),
		metric.WithUnit("ms"),
	); err != nil {
		log.Fatalf("metrics: %v", err)
	}
	if OutboundCommandProcessingTime, err = meter.Int64Histogram(
		"goblet.outbound_command_processing_time",
		metric.WithDescription("processing time of outbound commands"),
		metric.WithUnit("ms"),
	); err != nil {
		log.Fatalf("metrics: %v", err)
	}
	if UpstreamFetchWaitingTime, err = meter.Int64Histogram(
		"goblet.upstream_fetch_waiting_time",
		metric.WithDescription("waiting time of upstream fetch command"),
		metric.WithUnit("ms"),
	); err != nil {
		log.Fatalf("metrics: %v", err)
	}
}

// attrCtxKey is the context key under which we stash an attribute set.
type attrCtxKey struct{}

// withAttrs returns a context carrying the given attributes, replacing any
// existing entries for the same keys.
func withAttrs(ctx context.Context, kvs ...attribute.KeyValue) context.Context {
	existing := attrsFromCtx(ctx)
	merged := make(map[attribute.Key]attribute.KeyValue, len(existing)+len(kvs))
	for _, kv := range existing {
		merged[kv.Key] = kv
	}
	for _, kv := range kvs {
		merged[kv.Key] = kv
	}
	out := make([]attribute.KeyValue, 0, len(merged))
	for _, kv := range merged {
		out = append(out, kv)
	}
	return context.WithValue(ctx, attrCtxKey{}, out)
}

// attrsFromCtx returns the attributes attached to ctx, or nil.
func attrsFromCtx(ctx context.Context) []attribute.KeyValue {
	if v, ok := ctx.Value(attrCtxKey{}).([]attribute.KeyValue); ok {
		return v
	}
	return nil
}

// recordOpts builds metric.MeasurementOption from the context's attributes
// plus any extras.
func recordOpts(ctx context.Context, extras ...attribute.KeyValue) metric.MeasurementOption {
	attrs := append(attrsFromCtx(ctx), extras...)
	return metric.WithAttributes(attrs...)
}
