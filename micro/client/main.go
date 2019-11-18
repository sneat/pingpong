package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/golang/protobuf/ptypes"
	"github.com/micro/cli"
	"github.com/micro/go-micro"
	"github.com/micro/go-micro/client/selector/static"
	"github.com/micro/go-micro/registry/memory"
	_ "github.com/micro/go-plugins/registry/consul"
	"github.com/micro/go-plugins/wrapper/trace/opencensus"
	"github.com/sirupsen/logrus"
	pingpong "github.com/sneat/pingpong/proto"
	"github.com/sneat/pingpong/service"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
)

var (
	latency = stats.Float64("internal/latency", "The latency in seconds inside the server", "s")
)

func main() {
	log := logrus.New()

	var (
		jaegerAgent     string
		jaegerCollector string
		metricsListen   string
		serverAddress   string
	)
	// Micro client needs a service to support discovery
	svc := micro.NewService(
		micro.Name("pinger"),
		micro.Selector(static.NewSelector()),
		micro.Registry(memory.NewRegistry()),
		micro.WrapClient(opencensus.NewClientWrapper()),
		micro.WrapHandler(opencensus.NewHandlerWrapper()),
		micro.WrapSubscriber(opencensus.NewSubscriberWrapper()),
		micro.Flags(
			cli.StringFlag{
				Name:        "metrics_listen",
				Usage:       "The address to bind the metrics HTTP server to",
				EnvVar:      "METRICS_LISTEN",
				Value:       ":9889",
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
			cli.StringFlag{
				Name:        "destination_address",
				Usage:       "The server address to send micro requests to",
				EnvVar:      "DESTINATION_ADDRESS",
				Value:       "127.0.0.1:60051", // Run the server with `MICRO_SERVER_ADDRESS=":60051"`
				Destination: &serverAddress,
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
		if err := metricsSrv.Serve(metricsListener); err != nil {
			log.WithError(err).Fatalf("failed to serve metrics")
		}
		log.Infof("Metrics and pprof running on %s", metricsListener.Addr().String())
	}()

	c := pingpong.NewPongerService(serverAddress, svc.Client())

	log.WithField("address", serverAddress).Info("Sending micro ping requests")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Start the client service so that discovery works
		if err := svc.Run(); err != nil {
			log.Fatal(err)
		}
		cancel()
	}()

	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		i++
		r, err := sendPing(context.Background(), c)
		if err != nil {
			log.WithError(err).Fatal("could not ping")
		}
		start, err := ptypes.Timestamp(r.GetStart())
		if err != nil {
			log.WithError(err).WithField("start", r.GetStart()).Warn("could not convert response start time")
			continue
		}
		end, err := ptypes.Timestamp(r.GetEnd())
		if err != nil {
			log.WithError(err).WithField("end", r.GetEnd()).Warn("could not convert response end time")
			continue
		}
		roundTrip := start.Sub(end)
		if roundTrip > time.Second {
			log.WithField("response", r).WithField("duration", roundTrip.String()).Fatal("round trip call took too long")
		}
		writeLine(i)
		// Don't thrash the CPU too hard
		time.Sleep(10 * time.Millisecond)
	}
}

func sendPing(ctx context.Context, c pingpong.PongerService) (*pingpong.PingReply, error) {
	start := time.Now()
	ts, err := ptypes.TimestampProto(start)
	if err != nil {
		return nil, err
	}
	ctx, span := trace.StartSpan(ctx, "Ping")
	span.Annotate([]trace.Attribute{
		trace.StringAttribute("type", "micro"),
	}, "Sending ping")
	span.SetStatus(trace.Status{
		Message: "micro",
	})
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	defer span.End()
	defer stats.Record(ctx, latency.M(time.Since(start).Seconds()))
	return c.Ping(ctx, &pingpong.PingRequest{Start: ts})
}

func writeLine(i int) {
	previous := i - 1

	n := utf8.RuneCountInString(strconv.Itoa(previous))
	var clearString string
	for i := 0; i < n; i++ {
		clearString += " "
	}
	clearString += "\r"
	fmt.Printf("%s%d", clearString, i)
}
