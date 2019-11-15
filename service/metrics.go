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

type Metrics struct {
	pe              *prometheus.Exporter
	internalLatency *stats.Float64Measure
	externalLatency *stats.Float64Measure
}

func NewServiceMetrics(log logrus.FieldLogger, jaegerAgentEndpointURI, jaegerCollectorEndpointURI string) (*Metrics, error) {
	m := &Metrics{
		internalLatency: stats.Float64("internal/latency", "The latency in seconds inside the server", "s"),
		externalLatency: stats.Float64("external/latency", "The latency in seconds since the client request was created", "s"),
	}

	if err := view.Register(ocgrpc.DefaultServerViews...); err != nil {
		return nil, errors.Wrap(err, "could not register ocgrpc.DefaultServerViews")
	}

	internalLatencyView := &view.View{
		Name:        "internal/latency",
		Measure:     m.internalLatency,
		Description: "The distribution of the internal latencies",
		Aggregation: view.Distribution(0, 0.025, 0.05, 0.1, 0.2, 0.4, 0.8, 1, 2, 5, 10),
	}
	externalLatencyView := &view.View{
		Name:        "external/latency",
		Measure:     m.externalLatency,
		Description: "The distribution of the external latencies",
		Aggregation: view.Distribution(0, 0.025, 0.05, 0.1, 0.2, 0.4, 0.8, 1, 2, 5, 10),
	}
	if err := view.Register(internalLatencyView, externalLatencyView); err != nil {
		return nil, errors.Wrap(err, "could not register latency views")
	}

	je, err := jaeger.NewExporter(jaeger.Options{
		AgentEndpoint:     jaegerAgentEndpointURI,
		CollectorEndpoint: jaegerCollectorEndpointURI,
		Process: jaeger.Process{
			ServiceName: "ponger",
		},
	})
	if err != nil {
		log.Fatalf("Failed to create the Jaeger exporter: %v", err)
	}
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
	trace.RegisterExporter(je)

	m.pe, err = prometheus.NewExporter(prometheus.Options{
		Namespace: "ponger",
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not create Prometheus exporter")
	}

	return m, nil
}

func (m *Metrics) Exporter() *prometheus.Exporter {
	return m.pe
}

func (m *Metrics) InternalLatency() *stats.Float64Measure {
	return m.internalLatency
}

func (m *Metrics) ExternalLatency() *stats.Float64Measure {
	return m.externalLatency
}
