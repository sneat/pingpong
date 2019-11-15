//go:generate protoc -I ../proto --go_out=plugins=grpc:../proto --micro_out=../proto ../proto/pingpong.proto
package service

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
)

type Service struct {
	log              logrus.FieldLogger
	registrationType string
	metrics          *Metrics
}

func NewService(log logrus.FieldLogger, registrationType string, metrics *Metrics) *Service {
	return &Service{
		log: log.WithFields(logrus.Fields{
			"component":         "service",
			"registration_type": registrationType,
		}),
		registrationType: registrationType,
		metrics:          metrics,
	}
}

func (s *Service) Ping(ctx context.Context, requestTime time.Time) (time.Time, error) {
	ctx, span := trace.StartSpan(ctx, "Ping-response")
	span.Annotate([]trace.Attribute{
		trace.StringAttribute("type", s.registrationType),
	}, "Responding to ping")
	span.SetStatus(trace.Status{
		Message: s.registrationType,
	})
	defer span.End()

	stats.Record(ctx, s.metrics.internalLatency.M(time.Since(time.Now()).Seconds()), s.metrics.externalLatency.M(time.Since(requestTime).Seconds()))
	return time.Now(), nil
}
