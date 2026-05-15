package notification

import (
	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdknotification "github.com/dynatrace-oss/dtctl/sdk/api/notification"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Re-export SDK types so existing CLI code continues to compile unchanged.
type (
	EventNotification        = sdknotification.EventNotification
	EventNotificationList    = sdknotification.EventNotificationList
	ResourceNotification     = sdknotification.ResourceNotification
	ResourceNotificationList = sdknotification.ResourceNotificationList
)

// Handler handles notification resources.
// It delegates to the SDK handler.
type Handler struct {
	sdk *sdknotification.Handler
}

// NewHandler creates a new notification handler.
func NewHandler(c *client.Client) *Handler {
	return &Handler{
		sdk: sdknotification.NewHandler(httpclient.Wrap(c.HTTP())),
	}
}

// ListEventNotifications lists event notifications.
func (h *Handler) ListEventNotifications(notificationType string) (*EventNotificationList, error) {
	return h.sdk.ListEventNotifications(notificationType)
}

// GetEventNotification gets a specific event notification by ID.
func (h *Handler) GetEventNotification(id string) (*EventNotification, error) {
	return h.sdk.GetEventNotification(id)
}

// CreateEventNotification creates a new event notification.
func (h *Handler) CreateEventNotification(data []byte) (*EventNotification, error) {
	return h.sdk.CreateEventNotification(data)
}

// DeleteEventNotification deletes an event notification.
func (h *Handler) DeleteEventNotification(id string) error {
	return h.sdk.DeleteEventNotification(id)
}

// ListResourceNotifications lists resource notifications.
func (h *Handler) ListResourceNotifications(notificationType, resourceID string) (*ResourceNotificationList, error) {
	return h.sdk.ListResourceNotifications(notificationType, resourceID)
}

// GetResourceNotification gets a specific resource notification by ID.
func (h *Handler) GetResourceNotification(id string) (*ResourceNotification, error) {
	return h.sdk.GetResourceNotification(id)
}

// DeleteResourceNotification deletes a resource notification.
func (h *Handler) DeleteResourceNotification(id string) error {
	return h.sdk.DeleteResourceNotification(id)
}
