//go:build ignore
// +build ignore

// This file is intentionally excluded from the default build.
// To use Prometheus metrics, copy this file into your project and
// add the prometheus dependency:
//
//	go get github.com/prometheus/client_golang/prometheus
//
// Then replace endpoint.MetricsMiddleware with PrometheusMiddleware.
//
// Example usage (after copying):
//
//	import "github.com/prometheus/client_golang/prometheus"
//
//	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
//	    Name: "endpoint_requests_total",
//	    Help: "Total number of endpoint requests.",
//	}, []string{"operation", "status"})
//	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
//	    Name:    "endpoint_duration_seconds",
//	    Help:    "Endpoint request duration in seconds.",
//	    Buckets: prometheus.DefBuckets,
//	}, []string{"operation"})
//	prometheus.MustRegister(requests, duration)
//
//	ep = PrometheusMiddleware(requests, duration, "CreateUser")(ep)

package endpoint

// PrometheusMiddleware is a template showing how to integrate Prometheus.
// Copy this file into your project and uncomment after adding the dependency.
//
// func PrometheusMiddleware(
// 	requests *prometheus.CounterVec,
// 	duration *prometheus.HistogramVec,
// 	operation string,
// ) Middleware {
// 	return func(next Endpoint) Endpoint {
// 		return func(ctx context.Context, request any) (any, error) {
// 			start := time.Now()
// 			resp, err := next(ctx, request)
// 			status := "success"
// 			if err != nil {
// 				status = "error"
// 			}
// 			requests.WithLabelValues(operation, status).Inc()
// 			duration.WithLabelValues(operation).Observe(time.Since(start).Seconds())
// 			return resp, err
// 		}
// 	}
// }
