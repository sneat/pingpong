package main

import (
	"net"
	"net/http"
	"net/http/pprof"

	"github.com/micro/cli"
	"github.com/micro/go-micro"
	"github.com/micro/go-micro/broker"
	"github.com/micro/go-micro/registry/memory"
	_ "github.com/micro/go-plugins/registry/consul"
	"github.com/micro/go-plugins/wrapper/trace/opencensus"
	"github.com/sirupsen/logrus"
	"github.com/sneat/pingpong/proto"
	"github.com/sneat/pingpong/service"
	"go.opencensus.io/stats/view"
)

func main() {
	log := logrus.New()

	var (
		jaegerAgent     string
		jaegerCollector string
		metricsListen   string
	)
	svc := micro.NewService(
		micro.Name("pingpong"),
		micro.Registry(memory.NewRegistry()),
		micro.Broker(broker.NewBroker()),
		micro.WrapClient(opencensus.NewClientWrapper()),
		micro.WrapHandler(opencensus.NewHandlerWrapper()),
		micro.WrapSubscriber(opencensus.NewSubscriberWrapper()),
		micro.Flags(
			cli.StringFlag{
				Name:        "metrics_listen",
				Usage:       "The address to bind the metrics HTTP server to",
				EnvVar:      "METRICS_LISTEN",
				Value:       ":9888",
				Destination: &metricsListen,
			},
			cli.StringFlag{
				Name:        "jaeger_agent",
				Usage:       "The endpoint URI that the jaeger-agent is running on",
				EnvVar:      "JAEGER_AGENT",
				Value:       "localhost:6831",
				Destination: &jaegerCollector,
			},
			cli.StringFlag{
				Name:        "jaeger_collector",
				Usage:       "The endpoint URI that the jaeger-collector is running on",
				EnvVar:      "JAEGER_COLLECTOR",
				Value:       "http://localhost:14268/api/traces",
				Destination: &jaegerAgent,
			},
		),
	)
	svc.Init()

	metrics, err := service.NewServiceMetrics(log, jaegerAgent, jaegerCollector)
	if err != nil {
		log.WithError(err).Fatal("Could not create service metrics")
	}
	if err := view.Register(opencensus.DefaultServerViews...); err != nil {
		log.Fatal(err)
	}
	if err := view.Register(opencensus.DefaultClientViews...); err != nil {
		log.Fatal(err)
	}

	go func() {
		// Surface metrics and pprof
		mux := http.NewServeMux()
		mux.Handle("/metrics", metrics.Exporter())
		mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
		mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
		mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		metricsSrv := http.Server{
			Addr:    metricsListen,
			Handler: mux,
		}
		metricsListener, err := net.Listen("tcp", metricsSrv.Addr)
		if err != nil {
			log.WithError(err).WithField("addr", metricsSrv.Addr).Fatal("failed to listen")
		}
		metricsListen = metricsListener.Addr().String()
		if err := metricsSrv.Serve(metricsListener); err != nil {
			log.WithError(err).Fatalf("failed to serve metrics")
		}
	}()

	if err := pingpong.RegisterPongerHandler(svc.Server(), NewHandler(log, "micro", metrics)); err != nil {
		log.WithError(err).Fatal("Could not register micro Ponger Handler")
	}

	log.Printf("micro server listening on %s with metrics and pprof on %s", svc.Server().Options().Address, metricsListen)

	if err := svc.Run(); err != nil {
		log.Fatal(err)
	}
}
