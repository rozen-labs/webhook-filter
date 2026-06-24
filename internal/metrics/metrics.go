package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	RequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook_requests_total",
		Help: "Total webhook requests handled.",
	}, []string{"route", "status", "method"})
	AuthFailedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook_auth_failed_total",
		Help: "Total webhook auth failures.",
	}, []string{"route", "method"})
	FilteredTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook_filtered_total",
		Help: "Total filtered webhook requests.",
	}, []string{"route", "method"})
	ForwardedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook_forwarded_total",
		Help: "Total forwarded webhook requests.",
	}, []string{"route", "method"})
	ForwardErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook_forward_errors_total",
		Help: "Total webhook forwarding errors.",
	}, []string{"route", "method"})
	RequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "webhook_request_duration_seconds",
		Help:    "Webhook request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"route", "status", "method"})
	Registry = prometheus.NewRegistry()
)

func init() {
	Registry.MustRegister(RequestsTotal, AuthFailedTotal, FilteredTotal, ForwardedTotal, ForwardErrorsTotal, RequestDuration)
}
