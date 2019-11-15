package service

import (
	"contrib.go.opencensus.io/exporter/jaeger"
	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
)

type ClientMetrics struct {
	pe              *prometheus.Exporter
	internalLatency *stats.Float64Measure
}

func NewClientMetrics(log logrus.FieldLogger, jaegerAgentEndpointURI, jaegerCollectorEndpointURI string) (*ClientMetrics, error) {
	m := &ClientMetrics{
		internalLatency: stats.Float64("total/latency", "The latency in seconds for a round trip", "s"),
	}

	if err := view.Register(ocgrpc.DefaultServerViews...); err != nil {
		return nil, errors.Wrap(err, "could not register ocgrpc.DefaultServerViews")
	}

	internalLatencyView := &view.View{
		Name:        "total/latency",
		Measure:     m.internalLatency,
		Description: "The distribution of the total latencies",
		Aggregation: view.Distribution(0, 0.025, 0.05, 0.1, 0.2, 0.4, 0.8, 1, 2, 5, 10),
	}
	if err := view.Register(internalLatencyView); err != nil {
		return nil, errors.Wrap(err, "could not register latency views")
	}

	je, err := jaeger.NewExporter(jaeger.Options{
		AgentEndpoint:     jaegerAgentEndpointURI,
		CollectorEndpoint: jaegerCollectorEndpointURI,
		Process: jaeger.Process{
			ServiceName: "pinger",
		},
	})
	if err != nil {
		log.Fatalf("Failed to create the Jaeger exporter: %v", err)
	}
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
	trace.RegisterExporter(je)

	m.pe, err = prometheus.NewExporter(prometheus.Options{
		Namespace: "pinger",
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not create Prometheus exporter")
	}

	return m, nil
}

func (m *ClientMetrics) Exporter() *prometheus.Exporter {
	return m.pe
}
