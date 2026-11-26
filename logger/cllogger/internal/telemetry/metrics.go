// Package telemetry configure les métriques OpenTelemetry : export OTLP/HTTP
// (push) et un reader Prometheus (pull) pour le débogage local.
package telemetry

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"logger/internal/loadgen"
	"logger/internal/logstream"
)

// Init démarre le MeterProvider avec deux readers :
//   - OTLP/HTTP périodique (config via OTEL_EXPORTER_OTLP_*), le chemin principal ;
//   - Prometheus, exposé via le http.Handler renvoyé (à monter sur /metrics).
//
// La fonction renvoyée doit être appelée à l'arrêt pour flusher les dernières métriques.
func Init(ctx context.Context, serviceName, instanceID string, stream *logstream.Stream, gen *loadgen.Generator) (func(context.Context) error, http.Handler, error) {
	otlpExp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Registry dédié : n'expose que les métriques OTel, sans collecteurs Go/process.
	promReg := prometheus.NewRegistry()
	promExp, err := otelprom.New(otelprom.WithRegisterer(promReg))
	if err != nil {
		return nil, nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceInstanceID(instanceID),
		),
		resource.WithFromEnv(), // OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES surchargent les valeurs ci-dessus
	)
	if err != nil {
		return nil, nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(otlpExp, sdkmetric.WithInterval(15*time.Second))),
		sdkmetric.WithReader(promExp),
	)
	otel.SetMeterProvider(mp)

	if err := registerInstruments(mp.Meter(serviceName), stream, gen); err != nil {
		return nil, nil, err
	}

	handler := promhttp.HandlerFor(promReg, promhttp.HandlerOpts{})
	return mp.Shutdown, handler, nil
}

func registerInstruments(meter metric.Meter, stream *logstream.Stream, gen *loadgen.Generator) error {
	obs := func(v func() int64) metric.Int64Callback {
		return func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(v())
			return nil
		}
	}

	if _, err := meter.Int64ObservableCounter("logger.logs.emitted",
		metric.WithDescription("Logs mis en file d'attente."),
		metric.WithUnit("{log}"),
		metric.WithInt64Callback(obs(func() int64 { return int64(stream.Sent()) })),
	); err != nil {
		return err
	}

	if _, err := meter.Int64ObservableCounter("logger.logs.dropped",
		metric.WithDescription("Logs droppés car le buffer était plein."),
		metric.WithUnit("{log}"),
		metric.WithInt64Callback(obs(func() int64 { return int64(stream.Dropped()) })),
	); err != nil {
		return err
	}

	if _, err := meter.Int64ObservableUpDownCounter("logger.workers.active",
		metric.WithDescription("Goroutines de génération actives."),
		metric.WithUnit("{worker}"),
		metric.WithInt64Callback(obs(gen.ActiveWorkers)),
	); err != nil {
		return err
	}

	if _, err := meter.Int64ObservableGauge("logger.buffer.usage",
		metric.WithDescription("Logs actuellement dans le buffer."),
		metric.WithUnit("{log}"),
		metric.WithInt64Callback(obs(func() int64 { return int64(stream.Len()) })),
	); err != nil {
		return err
	}

	if _, err := meter.Int64ObservableGauge("logger.buffer.capacity",
		metric.WithDescription("Capacité totale du buffer."),
		metric.WithUnit("{log}"),
		metric.WithInt64Callback(obs(func() int64 { return int64(stream.Cap()) })),
	); err != nil {
		return err
	}

	if _, err := meter.Int64ObservableGauge("logger.output.mode",
		metric.WithDescription("Mode de sortie (0=stdout, 1=file, 2=both)."),
		metric.WithInt64Callback(obs(func() int64 { return int64(stream.Mode()) })),
	); err != nil {
		return err
	}

	return nil
}
