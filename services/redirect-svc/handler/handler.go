package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"go-micro.dev/v5"

	pb "github.com/go-systems-lab/go-url-shortener/proto/redirect"
	"github.com/go-systems-lab/go-url-shortener/services/redirect-svc/domain"
)

// RedirectHandler implements the redirect service gRPC interface
type RedirectHandler struct {
	service      *domain.RedirectService
	natsConn     *nats.Conn
	microService micro.Service
	serviceName  string
	version      string
}

// NewRedirectHandler creates a new redirect handler
func NewRedirectHandler(service *domain.RedirectService, natsConn *nats.Conn, microService micro.Service) *RedirectHandler {
	return &RedirectHandler{
		service:      service,
		natsConn:     natsConn,
		microService: microService,
		serviceName:  "redirect-service",
		version:      "v1.0.0",
	}
}

// ResolveURL resolves a short code to the original URL (main redirect functionality)
func (h *RedirectHandler) ResolveURL(ctx context.Context, req *pb.ResolveRequest, rsp *pb.ResolveResponse) error {
	// DEBUG: Log incoming request
	fmt.Printf("🔍 [DEBUG] ResolveURL called with shortCode: %s, clientIP: %s\n", req.ShortCode, req.ClientIp)

	// 1. Validate request
	if req.ShortCode == "" {
		fmt.Printf("❌ [DEBUG] Empty short code provided\n")
		rsp.Found = false
		rsp.Error = "Short code is required"
		return nil
	}

	// 2. Extract client information
	clientInfo := domain.ClientInfo{
		ClientIP:   req.ClientIp,
		UserAgent:  req.UserAgent,
		Referrer:   req.Referrer,
		Country:    req.Country,
		DeviceType: req.DeviceType,
	}

	// If client IP is empty, try to extract from context (gRPC metadata)
	if clientInfo.ClientIP == "" {
		clientInfo.ClientIP = h.extractClientIP(ctx)
	}

	fmt.Printf("🔍 [DEBUG] Calling service.ResolveURL for shortCode: %s\n", req.ShortCode)

	// 3. Resolve URL using business logic
	result, err := h.service.ResolveURL(ctx, req.ShortCode, clientInfo)
	if err != nil {
		fmt.Printf("❌ [DEBUG] service.ResolveURL failed: %v\n", err)
		rsp.Found = false
		rsp.Error = fmt.Sprintf("Internal error: %v", err)
		return nil
	}

	fmt.Printf("✅ [DEBUG] service.ResolveURL result: Found=%v, LongURL=%s, Error=%s\n", result.Found, result.LongURL, result.Error)

	// 4. If URL found and valid, track click event (async)
	if result.Found && !result.Expired {
		go h.publishClickEvent(req.ShortCode, result.LongURL, clientInfo)
	}

	// 5. Build response
	rsp.LongUrl = result.LongURL
	rsp.Found = result.Found
	rsp.Expired = result.Expired
	rsp.ClickCount = result.ClickCount
	rsp.Error = result.Error

	// Add timestamps if URL was found
	// if result.Found {
	// 	rsp.CreatedAt = result.CreatedAt.Unix()
	// 	if result.ExpiresAt != nil {
	// 		rsp.ExpiresAt = result.ExpiresAt.Unix()
	// 	}
	// }

	return nil
}

// TrackClick manually tracks a click event (for analytics service)
func (h *RedirectHandler) TrackClick(ctx context.Context, req *pb.ClickRequest, rsp *pb.ClickResponse) error {
	// 1. Validate request
	if req.ShortCode == "" || req.LongUrl == "" {
		rsp.Success = false
		rsp.Error = "Short code and long URL are required"
		return nil
	}

	// 2. Extract client information
	clientInfo := domain.ClientInfo{
		ClientIP:   req.ClientIp,
		UserAgent:  req.UserAgent,
		Referrer:   req.Referrer,
		Country:    req.Country,
		DeviceType: req.DeviceType,
	}

	// 3. Create click analytics data
	clickInfo, err := h.service.TrackClick(ctx, req.ShortCode, req.LongUrl, clientInfo)
	if err != nil {
		rsp.Success = false
		rsp.Error = fmt.Sprintf("Failed to process click: %v", err)
		return nil
	}

	// 4. Publish click event to NATS
	if err := h.publishClickEventFromInfo(clickInfo); err != nil {
		rsp.Success = false
		rsp.Error = fmt.Sprintf("Failed to publish click event: %v", err)
		return nil
	}

	rsp.Success = true
	return nil
}

// Health check endpoint
func (h *RedirectHandler) Health(ctx context.Context, req *pb.HealthRequest, rsp *pb.HealthResponse) error {
	rsp.Status = "ok"
	rsp.Service = h.serviceName
	rsp.Version = h.version
	rsp.Timestamp = time.Now().Unix()
	return nil
}

// publishClickEvent publishes click event to NATS via Go Micro broker for analytics
func (h *RedirectHandler) publishClickEvent(shortCode, longURL string, clientInfo domain.ClientInfo) {
	// Create click event data
	clickEvent := &pb.ClickEvent{
		ShortCode:  shortCode,
		LongUrl:    longURL,
		ClientIp:   clientInfo.ClientIP,
		UserAgent:  clientInfo.UserAgent,
		Referrer:   clientInfo.Referrer,
		Country:    clientInfo.Country,
		DeviceType: clientInfo.DeviceType,
		Timestamp:  time.Now().Unix(),
	}

	// Additional analytics data from domain service
	clickInfo, err := h.service.TrackClick(context.Background(), shortCode, longURL, clientInfo)
	if err == nil {
		clickEvent.City = clickInfo.City
		clickEvent.Browser = clickInfo.Browser
		clickEvent.Os = clickInfo.OS
		clickEvent.SessionId = clickInfo.SessionID
		clickEvent.IsUnique = clickInfo.IsUnique
	}

	// JSON marshal the event for proper transmission
	eventData, err := json.Marshal(clickEvent)
	if err != nil {
		fmt.Printf("Failed to marshal click event: %v\n", err)
		return
	}

	// Publish using Go Micro broker with JSON data
	message := h.microService.Client().NewMessage("url.clicked", eventData)
	if err := h.microService.Client().Publish(context.Background(), message); err != nil {
		fmt.Printf("Failed to publish click event to NATS: %v\n", err)
		return
	}

	fmt.Printf("✅ Published click event for %s to NATS (JSON: %s)\n", shortCode, string(eventData))
}

// publishClickEventFromInfo publishes click event from ClickInfo struct
func (h *RedirectHandler) publishClickEventFromInfo(clickInfo *domain.ClickInfo) error {
	// Create click event protobuf
	clickEvent := &pb.ClickEvent{
		ShortCode:  clickInfo.ShortCode,
		LongUrl:    clickInfo.LongURL,
		ClientIp:   clickInfo.ClientIP,
		UserAgent:  clickInfo.UserAgent,
		Referrer:   clickInfo.Referrer,
		Country:    clickInfo.Country,
		City:       clickInfo.City,
		DeviceType: clickInfo.DeviceType,
		Browser:    clickInfo.Browser,
		Os:         clickInfo.OS,
		Timestamp:  clickInfo.Timestamp.Unix(),
		SessionId:  clickInfo.SessionID,
		IsUnique:   clickInfo.IsUnique,
	}

	// JSON marshal the event for proper transmission
	eventData, err := json.Marshal(clickEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal click event: %w", err)
	}

	// Publish using Go Micro broker with JSON data
	message := h.microService.Client().NewMessage("url.clicked", eventData)
	if err := h.microService.Client().Publish(context.Background(), message); err != nil {
		return fmt.Errorf("failed to publish to NATS: %w", err)
	}

	return nil
}

// extractClientIP extracts client IP from gRPC context/metadata
func (h *RedirectHandler) extractClientIP(ctx context.Context) string {
	// Try to get IP from gRPC peer info
	if p, ok := ctx.Value("peer").(interface{ Addr() net.Addr }); ok {
		if addr := p.Addr(); addr != nil {
			if tcpAddr, ok := addr.(*net.TCPAddr); ok {
				return tcpAddr.IP.String()
			}
		}
	}

	// Fallback: try common header patterns
	// In real implementation, you'd extract from gRPC metadata
	return "unknown"
}

// Helper function to sanitize client IP
func (h *RedirectHandler) sanitizeClientIP(ip string) string {
	// Remove port if present
	if strings.Contains(ip, ":") {
		host, _, err := net.SplitHostPort(ip)
		if err == nil {
			return host
		}
	}
	return ip
}

// GetRedirectStats returns redirect service statistics (extension method)
func (h *RedirectHandler) GetRedirectStats(ctx context.Context, shortCode string) (map[string]interface{}, error) {
	return h.service.GetURLStats(ctx, shortCode)
}

// PrewarmCache preloads popular URLs into cache (extension method)
func (h *RedirectHandler) PrewarmCache(ctx context.Context, shortCodes []string) error {
	return h.service.PrewarmPopularURLs(ctx, shortCodes)
}

// InvalidateURL removes a URL from cache (extension method)
func (h *RedirectHandler) InvalidateURL(ctx context.Context, shortCode string) error {
	return h.service.InvalidateURL(ctx, shortCode)
}
