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
	"github.com/micro/go-micro"
	_ "github.com/micro/go-plugins/registry/consul"
	"github.com/micro/go-plugins/wrapper/trace/opencensus"
	"github.com/sirupsen/logrus"
	pingpong "github.com/sneat/pingpong/proto"
	"github.com/sneat/pingpong/service"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
)

var (
	latency = stats.Float64("internal/latency", "The latency in seconds inside the server", "s")
)

func main() {
	log := logrus.New()

	pflag.String("micro_address", "127.0.0.1:60051", "The server address to send gRPC requests to")
	pflag.String("metrics_listen", ":9889", "The address to bind the metrics HTTP server to")
	pflag.String("jaeger_agent", "localhost:6831", "The endpoint URI that the jaeger-agent is running on")
	pflag.String("jaeger_collector", "http://localhost:14268/api/traces", "The endpoint URI that the jaeger-collector is running on")
	pflag.Parse()
	viper.AutomaticEnv()
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.WithError(err).Fatal("Could not bind viper command line flags")
	}
	log.WithField("viper", viper.AllSettings()).Info("All viper settings")

	metrics, err := service.NewClientMetrics(log, viper.GetString("jaeger_agent"), viper.GetString("jaeger_collector"))
	if err != nil {
		log.WithError(err).Fatal("Could not create service metrics")
	}
	if err := view.Register(opencensus.DefaultServerViews...); err != nil {
		log.Fatal(err)
	}
	if err := view.Register(opencensus.DefaultClientViews...); err != nil {
		log.Fatal(err)
	}

	metricsAddr := viper.GetString("metrics_listen")
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
			Addr:    metricsAddr,
			Handler: mux,
		}
		metricsListener, err := net.Listen("tcp", metricsSrv.Addr)
		if err != nil {
			log.WithError(err).WithField("addr", metricsSrv.Addr).Fatal("failed to listen")
		}
		metricsAddr = metricsListener.Addr().String()
		if err := metricsSrv.Serve(metricsListener); err != nil {
			log.WithError(err).Fatalf("failed to serve metrics")
		}
	}()
	log.Infof("Metrics and pprof running on %s", metricsAddr)

	// Micro client needs a service to support discovery
	svc := micro.NewService(
		micro.Name("pinger"),
		micro.WrapClient(opencensus.NewClientWrapper()),
		micro.WrapHandler(opencensus.NewHandlerWrapper()),
		micro.WrapSubscriber(opencensus.NewSubscriberWrapper()),
	)
	svc.Init()
	c := pingpong.NewPongerService("pingpong", svc.Client())

	log.WithField("address", viper.GetString("micro_address")).Info("Sending micro ping requests")

	go func() {
		// Start the client service so that discovery works
		if err := svc.Run(); err != nil {
			log.Fatal(err)
		}
	}()

	i := 0
	for {
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
