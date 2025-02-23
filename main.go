package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"

	"github.com/gofiber/contrib/otelfiber"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.uber.org/zap"

	"microservicetest/app/healthcheck"
	"microservicetest/app/product"
	"microservicetest/infra/couchbase"
	"microservicetest/pkg/config"
	_ "microservicetest/pkg/log"
)

// Prometheus metrics
var httpRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "http_request_duration_seconds",
	Help:    "Duration of HTTP requests in seconds",
	Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
}, []string{"route", "method", "status"})

func init() {
	prometheus.MustRegister(httpRequestDuration)
}

// RequestDurationMiddleware measures the duration of HTTP requests
func RequestDurationMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Call next handler
		err := c.Next()

		// Record duration after the handler returns
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Response().StatusCode())

		// Record metrics
		httpRequestDuration.WithLabelValues(
			c.Route().Path,
			c.Method(),
			status,
		).Observe(duration)

		return err
	}
}

type Request any
type Response any

// Define an interface for handlers
type HandlerInterface[R Request, Res Response] interface {
	Handle(ctx context.Context, req *R) (*Res, error)
}

// Update handle function to accept HandlerInterface instead of Handler function
func handle[R Request, Res Response](handler HandlerInterface[R, Res]) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req R

		if err := c.BodyParser(&req); err != nil && !errors.Is(err, fiber.ErrUnprocessableEntity) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		if err := c.ParamsParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		if err := c.QueryParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		if err := c.ReqHeaderParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		/*
			ctx, cancel := context.WithTimeout(c.UserContext(), 3*time.Second)
			defer cancel()
		*/

		ctx := c.UserContext()

		res, err := handler.Handle(ctx, &req)
		if err != nil {
			zap.L().Error("Failed to handle request", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(res)
	}
}

func main() {
	appConfig := config.Read()
	defer zap.L().Sync()
	zap.L().Info("app starting...")
	zap.L().Info("app config", zap.Any("appConfig", appConfig))

	tp := initTracer(appConfig)
	client := httpc()

	retryClient := retryablehttp.NewClient()
	retryClient.HTTPClient.Transport = client.Transport
	retryClient.RetryMax = 0
	retryClient.RetryWaitMin = 100 * time.Millisecond
	retryClient.RetryWaitMax = 10 * time.Second
	retryClient.Backoff = retryablehttp.LinearJitterBackoff
	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	}

	couchbaseRepository := couchbase.NewCouchbaseRepository(tp, appConfig.CouchbaseUrl, appConfig.CouchbaseUsername, appConfig.CouchbasePassword)

	getProductHandler := product.NewGetProductHandler(couchbaseRepository, retryClient, appConfig.HttpServer)
	createProductHandler := product.NewCreateProductHandler(couchbaseRepository)
	healthcheckHandler := healthcheck.NewHealthCheckHandler()

	app := fiber.New(fiber.Config{
		IdleTimeout:  5 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Concurrency:  256 * 1024,
	})

	app.Use(otelfiber.Middleware())
	app.Use(RequestDurationMiddleware())

	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))
	app.Get("/healthcheck", handle[healthcheck.HealthCheckRequest, healthcheck.HealthCheckResponse](healthcheckHandler))

	app.Get("/products/:id", handle[product.GetProductRequest, product.GetProductResponse](getProductHandler))
	app.Post("/products", handle[product.CreateProductRequest, product.CreateProductResponse](createProductHandler))

	// Start server in a goroutine
	go func() {
		if err := app.Listen(fmt.Sprintf("0.0.0.0:%s", appConfig.Port)); err != nil {
			zap.L().Error("Failed to start server", zap.Error(err))
			os.Exit(1)
		}
	}()

	zap.L().Info("Server started on port", zap.String("port", appConfig.Port))

	gracefulShutdown(app)
}

func gracefulShutdown(app *fiber.App) {
	// Create channel for shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal
	<-sigChan
	zap.L().Info("Shutting down server...")

	// Shutdown with 5 second timeout
	if err := app.ShutdownWithTimeout(5 * time.Second); err != nil {
		zap.L().Error("Error during server shutdown", zap.Error(err))
	}

	zap.L().Info("Server gracefully stopped")
}

func httpc() *http.Client {
	transport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{
		Transport: otelhttp.NewTransport(transport),
	}

	return httpClient
}

func initTracer(appConfig *config.AppConfig) *sdktrace.TracerProvider {
	headers := map[string]string{
		"content-type": "application/json",
	}

	exporter, err := otlptrace.New(
		context.Background(),
		otlptracehttp.NewClient(
			otlptracehttp.WithEndpoint(appConfig.OtelTraceEndpoint),
			otlptracehttp.WithHeaders(headers),
			otlptracehttp.WithInsecure(),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String("microservice-go"),
			)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp
}
