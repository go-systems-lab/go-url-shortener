package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	pb "github.com/go-systems-lab/go-url-shortener/proto/url"
	"github.com/go-systems-lab/go-url-shortener/services/url-shortener-svc/store"
	"github.com/go-systems-lab/go-url-shortener/utils/cache"
	"github.com/go-systems-lab/go-url-shortener/utils/database"
)

// URLHandler implements the Go Micro URLShortenerHandler interface
type URLHandler struct {
	store *store.URLStore
	log   *logrus.Logger
}

// NewURLHandler creates a new URL handler instance
func NewURLHandler(db *database.PostgreSQL, cache *cache.Redis) pb.URLShortenerHandler {
	urlStore := store.NewURLStore(db, cache)
	return &URLHandler{
		store: urlStore,
		log:   logrus.New(),
	}
}

// ShortenURL implements the ShortenURL RPC method
func (h *URLHandler) ShortenURL(ctx context.Context, req *pb.ShortenRequest, rsp *pb.ShortenResponse) error {
	h.log.WithFields(logrus.Fields{
		"long_url": req.LongUrl,
		"user_id":  req.UserId,
	}).Info("Processing ShortenURL request")

	// Convert protobuf request to store request
	storeReq := &store.ShortenURLRequest{
		LongURL:     req.LongUrl,
		CustomAlias: req.CustomAlias,
		UserID:      req.UserId,
		Metadata:    req.Metadata,
	}

	// Handle expiration time
	if req.ExpirationTime > 0 {
		expirationTime := time.Unix(req.ExpirationTime, 0)
		storeReq.ExpirationTime = &expirationTime
	}

	// Call store layer
	urlResponse, err := h.store.ShortenURL(storeReq)
	if err != nil {
		h.log.WithError(err).Error("Failed to shorten URL")
		return fmt.Errorf("failed to shorten URL: %w", err)
	}

	// Convert store response to protobuf response
	rsp.ShortCode = urlResponse.ShortCode
	rsp.ShortUrl = fmt.Sprintf("https://short.ly/%s", urlResponse.ShortCode) // TODO: Make configurable
	rsp.LongUrl = urlResponse.LongURL
	rsp.CreatedAt = urlResponse.CreatedAt.Unix()
	rsp.UserId = urlResponse.UserID

	if urlResponse.ExpiresAt != nil {
		rsp.ExpiresAt = urlResponse.ExpiresAt.Unix()
	}

	h.log.WithFields(logrus.Fields{
		"short_code": rsp.ShortCode,
		"user_id":    rsp.UserId,
	}).Info("URL shortened successfully")

	return nil
}

// GetURLInfo implements the GetURLInfo RPC method
func (h *URLHandler) GetURLInfo(ctx context.Context, req *pb.GetURLRequest, rsp *pb.URLInfo) error {
	h.log.WithFields(logrus.Fields{
		"short_code": req.ShortCode,
		"user_id":    req.UserId,
	}).Info("Processing GetURLInfo request")

	// Call store layer
	urlResponse, err := h.store.GetURL(req.ShortCode, req.UserId)
	if err != nil {
		h.log.WithError(err).Error("Failed to get URL info")
		return fmt.Errorf("failed to get URL info: %w", err)
	}

	// Convert store response to protobuf response
	rsp.ShortCode = urlResponse.ShortCode
	rsp.ShortUrl = fmt.Sprintf("https://short.ly/%s", urlResponse.ShortCode) // TODO: Make configurable
	rsp.LongUrl = urlResponse.LongURL
	rsp.UserId = urlResponse.UserID
	rsp.CreatedAt = urlResponse.CreatedAt.Unix()
	rsp.ClickCount = urlResponse.ClickCount
	rsp.IsActive = urlResponse.IsActive
	rsp.Metadata = urlResponse.Metadata

	if urlResponse.ExpiresAt != nil {
		rsp.ExpiresAt = urlResponse.ExpiresAt.Unix()
	}

	h.log.WithFields(logrus.Fields{
		"short_code": rsp.ShortCode,
		"user_id":    rsp.UserId,
	}).Info("URL info retrieved successfully")

	return nil
}

// DeleteURL implements the DeleteURL RPC method
func (h *URLHandler) DeleteURL(ctx context.Context, req *pb.DeleteURLRequest, rsp *pb.DeleteResponse) error {
	h.log.WithFields(logrus.Fields{
		"short_code": req.ShortCode,
		"user_id":    req.UserId,
	}).Info("Processing DeleteURL request")

	// Call store layer
	err := h.store.DeleteURL(req.ShortCode, req.UserId)
	if err != nil {
		h.log.WithError(err).Error("Failed to delete URL")
		rsp.Success = false
		rsp.Message = fmt.Sprintf("Failed to delete URL: %v", err)
		return nil
	}

	rsp.Success = true
	rsp.Message = "URL deleted successfully"

	h.log.WithFields(logrus.Fields{
		"short_code": req.ShortCode,
		"user_id":    req.UserId,
	}).Info("URL deleted successfully")

	return nil
}

// GetUserURLs implements the GetUserURLs RPC method
func (h *URLHandler) GetUserURLs(ctx context.Context, req *pb.GetUserURLsRequest, rsp *pb.GetUserURLsResponse) error {
	h.log.WithFields(logrus.Fields{
		"user_id":   req.UserId,
		"page":      req.Page,
		"page_size": req.PageSize,
	}).Info("Processing GetUserURLs request")

	// Convert protobuf request to store request
	storeReq := &store.GetUserURLsRequest{
		UserID:    req.UserId,
		Page:      req.Page,
		PageSize:  req.PageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	// Call store layer
	storeResponse, err := h.store.GetUserURLs(storeReq)
	if err != nil {
		h.log.WithError(err).Error("Failed to get user URLs")
		return fmt.Errorf("failed to get user URLs: %w", err)
	}

	// Convert store URLs to protobuf URLs
	urls := make([]*pb.URLInfo, len(storeResponse.URLs))
	for i, storeURL := range storeResponse.URLs {
		urlInfo := &pb.URLInfo{
			ShortCode:  storeURL.ShortCode,
			ShortUrl:   fmt.Sprintf("https://short.ly/%s", storeURL.ShortCode), // TODO: Make configurable
			LongUrl:    storeURL.LongURL,
			UserId:     storeURL.UserID,
			CreatedAt:  storeURL.CreatedAt.Unix(),
			ClickCount: storeURL.ClickCount,
			IsActive:   storeURL.IsActive,
			Metadata:   storeURL.Metadata,
		}

		if storeURL.ExpiresAt != nil {
			urlInfo.ExpiresAt = storeURL.ExpiresAt.Unix()
		}

		urls[i] = urlInfo
	}

	rsp.Urls = urls
	rsp.TotalCount = storeResponse.TotalCount
	rsp.Page = storeResponse.Page
	rsp.PageSize = storeResponse.PageSize
	rsp.HasNext = storeResponse.HasNext

	h.log.WithFields(logrus.Fields{
		"user_id":   req.UserId,
		"url_count": len(urls),
		"total":     rsp.TotalCount,
	}).Info("User URLs retrieved successfully")

	return nil
}

// UpdateURL implements the UpdateURL RPC method
func (h *URLHandler) UpdateURL(ctx context.Context, req *pb.UpdateURLRequest, rsp *pb.UpdateURLResponse) error {
	h.log.WithFields(logrus.Fields{
		"short_code": req.ShortCode,
		"user_id":    req.UserId,
	}).Info("Processing UpdateURL request")

	// Convert protobuf request to store request
	storeReq := &store.UpdateURLRequest{
		ShortCode:  req.ShortCode,
		UserID:     req.UserId,
		NewLongURL: req.NewLongUrl,
		Metadata:   req.Metadata,
	}

	// Handle new expiration time
	if req.NewExpirationTime > 0 {
		newExpirationTime := time.Unix(req.NewExpirationTime, 0)
		storeReq.NewExpirationTime = &newExpirationTime
	}

	// Call store layer
	urlResponse, err := h.store.UpdateURL(storeReq)
	if err != nil {
		h.log.WithError(err).Error("Failed to update URL")
		rsp.Success = false
		rsp.Message = fmt.Sprintf("Failed to update URL: %v", err)
		return nil
	}

	// Convert store response to protobuf response
	updatedURL := &pb.URLInfo{
		ShortCode:  urlResponse.ShortCode,
		ShortUrl:   fmt.Sprintf("https://short.ly/%s", urlResponse.ShortCode), // TODO: Make configurable
		LongUrl:    urlResponse.LongURL,
		UserId:     urlResponse.UserID,
		CreatedAt:  urlResponse.CreatedAt.Unix(),
		ClickCount: urlResponse.ClickCount,
		IsActive:   urlResponse.IsActive,
		Metadata:   urlResponse.Metadata,
	}

	if urlResponse.ExpiresAt != nil {
		updatedURL.ExpiresAt = urlResponse.ExpiresAt.Unix()
	}

	rsp.Success = true
	rsp.Message = "URL updated successfully"
	rsp.UpdatedUrl = updatedURL

	h.log.WithFields(logrus.Fields{
		"short_code": req.ShortCode,
		"user_id":    req.UserId,
	}).Info("URL updated successfully")

	return nil
}
