// Code generated by protoc-gen-micro. DO NOT EDIT.
// source: proto/analytics/analytics.proto

package analytics

import (
	fmt "fmt"
	proto "google.golang.org/protobuf/proto"
	math "math"
)

import (
	context "context"
	client "go-micro.dev/v5/client"
	server "go-micro.dev/v5/server"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ client.Option
var _ server.Option

// Client API for AnalyticsService service

type AnalyticsService interface {
	// Process click event (async from NATS)
	ProcessClick(ctx context.Context, in *ClickEvent, opts ...client.CallOption) (*ProcessResponse, error)
	// Get URL statistics
	GetURLStats(ctx context.Context, in *StatsRequest, opts ...client.CallOption) (*StatsResponse, error)
	// Get top URLs
	GetTopURLs(ctx context.Context, in *TopURLsRequest, opts ...client.CallOption) (*TopURLsResponse, error)
	// Get analytics dashboard data
	GetDashboard(ctx context.Context, in *DashboardRequest, opts ...client.CallOption) (*DashboardResponse, error)
	// Health check
	Health(ctx context.Context, in *HealthRequest, opts ...client.CallOption) (*HealthResponse, error)
}

type analyticsService struct {
	c    client.Client
	name string
}

func NewAnalyticsService(name string, c client.Client) AnalyticsService {
	return &analyticsService{
		c:    c,
		name: name,
	}
}

func (c *analyticsService) ProcessClick(ctx context.Context, in *ClickEvent, opts ...client.CallOption) (*ProcessResponse, error) {
	req := c.c.NewRequest(c.name, "AnalyticsService.ProcessClick", in)
	out := new(ProcessResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *analyticsService) GetURLStats(ctx context.Context, in *StatsRequest, opts ...client.CallOption) (*StatsResponse, error) {
	req := c.c.NewRequest(c.name, "AnalyticsService.GetURLStats", in)
	out := new(StatsResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *analyticsService) GetTopURLs(ctx context.Context, in *TopURLsRequest, opts ...client.CallOption) (*TopURLsResponse, error) {
	req := c.c.NewRequest(c.name, "AnalyticsService.GetTopURLs", in)
	out := new(TopURLsResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *analyticsService) GetDashboard(ctx context.Context, in *DashboardRequest, opts ...client.CallOption) (*DashboardResponse, error) {
	req := c.c.NewRequest(c.name, "AnalyticsService.GetDashboard", in)
	out := new(DashboardResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *analyticsService) Health(ctx context.Context, in *HealthRequest, opts ...client.CallOption) (*HealthResponse, error) {
	req := c.c.NewRequest(c.name, "AnalyticsService.Health", in)
	out := new(HealthResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for AnalyticsService service

type AnalyticsServiceHandler interface {
	// Process click event (async from NATS)
	ProcessClick(context.Context, *ClickEvent, *ProcessResponse) error
	// Get URL statistics
	GetURLStats(context.Context, *StatsRequest, *StatsResponse) error
	// Get top URLs
	GetTopURLs(context.Context, *TopURLsRequest, *TopURLsResponse) error
	// Get analytics dashboard data
	GetDashboard(context.Context, *DashboardRequest, *DashboardResponse) error
	// Health check
	Health(context.Context, *HealthRequest, *HealthResponse) error
}

func RegisterAnalyticsServiceHandler(s server.Server, hdlr AnalyticsServiceHandler, opts ...server.HandlerOption) error {
	type analyticsService interface {
		ProcessClick(ctx context.Context, in *ClickEvent, out *ProcessResponse) error
		GetURLStats(ctx context.Context, in *StatsRequest, out *StatsResponse) error
		GetTopURLs(ctx context.Context, in *TopURLsRequest, out *TopURLsResponse) error
		GetDashboard(ctx context.Context, in *DashboardRequest, out *DashboardResponse) error
		Health(ctx context.Context, in *HealthRequest, out *HealthResponse) error
	}
	type AnalyticsService struct {
		analyticsService
	}
	h := &analyticsServiceHandler{hdlr}
	return s.Handle(s.NewHandler(&AnalyticsService{h}, opts...))
}

type analyticsServiceHandler struct {
	AnalyticsServiceHandler
}

func (h *analyticsServiceHandler) ProcessClick(ctx context.Context, in *ClickEvent, out *ProcessResponse) error {
	return h.AnalyticsServiceHandler.ProcessClick(ctx, in, out)
}

func (h *analyticsServiceHandler) GetURLStats(ctx context.Context, in *StatsRequest, out *StatsResponse) error {
	return h.AnalyticsServiceHandler.GetURLStats(ctx, in, out)
}

func (h *analyticsServiceHandler) GetTopURLs(ctx context.Context, in *TopURLsRequest, out *TopURLsResponse) error {
	return h.AnalyticsServiceHandler.GetTopURLs(ctx, in, out)
}

func (h *analyticsServiceHandler) GetDashboard(ctx context.Context, in *DashboardRequest, out *DashboardResponse) error {
	return h.AnalyticsServiceHandler.GetDashboard(ctx, in, out)
}

func (h *analyticsServiceHandler) Health(ctx context.Context, in *HealthRequest, out *HealthResponse) error {
	return h.AnalyticsServiceHandler.Health(ctx, in, out)
}
