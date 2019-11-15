package main

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/sirupsen/logrus"
	"github.com/sneat/pingpong/proto"
	"github.com/sneat/pingpong/service"
)

type handler struct {
	log     logrus.FieldLogger
	service *service.Service
}

func NewHandler(log logrus.FieldLogger, registrationType string, metrics *service.Metrics) *handler {
	return &handler{
		log:     log.WithField("component", "handler"),
		service: service.NewService(log, registrationType, metrics),
	}
}

func (h *handler) Ping(ctx context.Context, req *pingpong.PingRequest, rsp *pingpong.PingReply) error {
	start := time.Now()
	clientStart, err := ptypes.Timestamp(req.GetStart())
	if err != nil {
		h.log.WithError(err).WithFields(logrus.Fields{
			"in":          req.GetStart(),
			"replacement": start,
		}).Warn("Could not convert input time, replacing")
		clientStart = start
		ts, err := ptypes.TimestampProto(clientStart)
		if err != nil {
			h.log.WithError(err).WithFields(logrus.Fields{
				"in": clientStart,
			}).Warn("Could not convert start time back to proto type")
		}
		req.Start = ts
	}
	r, err := h.service.Ping(ctx, clientStart)
	if err != nil {
		return err
	}
	ts, err := ptypes.TimestampProto(r)
	if err != nil {
		h.log.WithError(err).WithFields(logrus.Fields{
			"in": r,
		}).Warn("Could not convert end time back to proto type")
	}
	rsp.Start = req.GetStart()
	rsp.End = ts

	return nil
}
