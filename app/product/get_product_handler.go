package product

import (
	"context"
	"io"
	"microservicetest/domain"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

type GetProductRequest struct {
	ID string `json:"id" param:"id"`
}

type GetProductResponse struct {
	Product *domain.Product `json:"product"`
}

type GetProductHandler struct {
	repository Repository
	httpClient *retryablehttp.Client
	breaker    *gobreaker.CircuitBreaker
}

func NewGetProductHandler(repository Repository, httpClient *retryablehttp.Client) *GetProductHandler {
	// Configure the circuit breaker
	breakerSettings := gobreaker.Settings{
		Name:        "http-client",
		MaxRequests: 3,                // Number of requests allowed in half-open state
		Interval:    5 * time.Second,  // Time window for counting failures
		Timeout:     10 * time.Second, // Time to wait before switching from open to half-open
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 3 && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			zap.L().Info("Circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()))
			// You could add logging here
		},
	}

	return &GetProductHandler{
		repository: repository,
		httpClient: httpClient,
		breaker:    gobreaker.NewCircuitBreaker(breakerSettings),
	}
}

func (h *GetProductHandler) Handle(ctx context.Context, req *GetProductRequest) (*GetProductResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8081/random-error", nil)
	if err != nil {
		return nil, err
	}

	retryableReq, err := retryablehttp.FromRequest(httpReq)
	if err != nil {
		return nil, err
	}

	// Execute the HTTP request through the circuit breaker
	resp, err := h.breaker.Execute(func() (interface{}, error) {
		return h.httpClient.Do(retryableReq)
	})
	if err != nil {
		return nil, err
	}

	httpResp := resp.(*http.Response)
	defer httpResp.Body.Close()
	if _, err = io.ReadAll(httpResp.Body); err != nil {
		return nil, err
	}

	product, err := h.repository.GetProduct(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	return &GetProductResponse{Product: product}, nil
}
