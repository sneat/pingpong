package main

import (
	"net"
	"net/http"
	"net/http/pprof"

	"github.com/sirupsen/logrus"
	"github.com/sneat/pingpong/proto"
	"github.com/sneat/pingpong/service"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
)

func main() {
	log := logrus.New()

	pflag.String("grpc_listen", ":50051", "The address to bind the gRPC server to")
	pflag.String("metrics_listen", ":8888", "The address to bind the metrics HTTP server to")
	pflag.String("jaeger_agent", "localhost:6831", "The endpoint URI that the jaeger-agent is running on")
	pflag.String("jaeger_collector", "http://localhost:14268/api/traces", "The endpoint URI that the jaeger-collector is running on")
	pflag.Parse()
	viper.AutomaticEnv()
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.WithError(err).Fatal("Could not bind viper command line flags")
	}
	log.WithField("viper", viper.AllSettings()).Info("All viper settings")

	metrics, err := service.NewServiceMetrics(log, viper.GetString("jaeger_agent"), viper.GetString("jaeger_collector"))
	if err != nil {
		log.WithError(err).Fatal("Could not create service metrics")
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

	lis, err := net.Listen("tcp", viper.GetString("grpc_listen"))
	if err != nil {
		log.WithError(err).WithField("addr", viper.GetString("grpc_listen")).Fatal("failed to listen")
	}
	s := grpc.NewServer(grpc.StatsHandler(&ocgrpc.ServerHandler{}))
	pingpong.RegisterPongerServer(s, NewHandler(log, "gRPC", metrics))

	log.Printf("gRPC server listening on %s with metrics and pprof on %s", lis.Addr(), metricsAddr)

	if err := s.Serve(lis); err != nil {
		log.WithError(err).Fatalf("failed to serve gRPC")
	}
}
