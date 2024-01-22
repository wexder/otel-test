package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

func main() {
	spanCount := flag.Int("spanCount", 100, "Span count to produce")
	parallelCount := flag.Int("parallelCount", 100, "Paralel traces")
	spanDuration := flag.Duration("spanDuration", 10*time.Millisecond, "Span duration")
	flag.Parse()

	tracerConfig := loadTracerConfig()
	traceProvider, err := TracerProvider(tracerConfig)
	if err != nil {
		panic(err)
	}
	tracer := traceProvider.Tracer("Test")

	wg := sync.WaitGroup{}
	for i := 0; i < *parallelCount; i++ {
		i := i
		rootCtx, rootSpan := tracer.Start(context.Background(), "Root")
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < *spanCount; j++ {
				slog.Info(fmt.Sprintf("Span %d %d", i, j))
				j := j
				ctx, span := tracer.Start(rootCtx, fmt.Sprintf("Test_%d_%d", i, j))
				time.Sleep(*spanDuration)
				span.End()
				rootCtx = ctx
			}

			rootSpan.End()
		}()
	}

	wg.Wait()
	err = traceProvider.ForceFlush(context.Background())
	if err != nil {
		panic(err)
	}
}

func configViper(configName string) *viper.Viper {
	v := viper.New()
	v.SetConfigName(configName)
	v.SetConfigType("yaml")

	v.AddConfigPath(".")
	return v
}

func loadTracerConfig() TracerConfig {
	tracerConfig := &TracerConfig{}
	v := configViper("tracer")
	err := v.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %w \n", err))
	}
	err = v.Unmarshal(tracerConfig)
	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %w \n", err))
	}
	return *tracerConfig
}

type TracerConfig struct {
	Endpoint string
	TLS      bool
}

func TracerProvider(config TracerConfig) (*tracesdk.TracerProvider, error) {
	ctx := context.Background()

	options := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(config.Endpoint),
	}
	if !config.TLS {
		options = append(options, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, options...)
	if err != nil {
		return nil, err
	}

	sampler := tracesdk.ParentBased(tracesdk.TraceIDRatioBased(1))

	bsp := tracesdk.NewBatchSpanProcessor(exporter)
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSpanProcessor(bsp),
		tracesdk.WithSampler(sampler),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("test"),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil
}
