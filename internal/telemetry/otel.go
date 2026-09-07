package telemetry

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const TracerName = "llm-gateway"

// Setup initializes the global OTel TracerProvider and W3C TextMapPropagator.
// Returns a shutdown function to flush spans on graceful exit.
func Setup(ctx context.Context, serviceName, exporterType, otlpEndpoint string) (func(context.Context) error, error) {
	if exporterType == "" {
		exporterType = os.Getenv("OTEL_EXPORTER")
		if exporterType == "" {
			exporterType = "otlp"
		}
	}

	if otlpEndpoint == "" {
		otlpEndpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		if otlpEndpoint == "" {
			otlpEndpoint = "localhost:4317"
		}
	}

	// Always install W3C TraceContext & Baggage propagators
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if exporterType == "none" || exporterType == "disabled" {
		log.Println("OTel tracing is disabled (noop tracer installed)")
		return func(context.Context) error { return nil }, nil
	}

	var exp sdktrace.SpanExporter
	var err error

	switch exporterType {
	case "stdout":
		exp, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("failed creating stdout trace exporter: %w", err)
		}
	case "otlp":
		endpoint := otlpEndpoint
		// Strip scheme if present (e.g. http://localhost:4317 -> localhost:4317)
		if u, parseErr := url.Parse(endpoint); parseErr == nil && u.Host != "" {
			endpoint = u.Host
		} else {
			endpoint = strings.TrimPrefix(endpoint, "http://")
			endpoint = strings.TrimPrefix(endpoint, "https://")
		}

		exp, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithEndpoint(endpoint),
		)
		if err != nil {
			return nil, fmt.Errorf("failed creating OTLP gRPC trace exporter: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown OTEL_EXPORTER: %q (expected otlp, stdout, or none)", exporterType)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed creating trace resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	log.Printf("OTel tracing enabled: exporter=%s, endpoint=%s", exporterType, otlpEndpoint)
	return tp.Shutdown, nil
}
