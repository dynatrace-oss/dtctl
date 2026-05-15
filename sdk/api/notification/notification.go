package notification

import (
	"encoding/json"
	"fmt"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Handler handles notification resources.
type Handler struct {
	client *httpclient.Client
}

// NewHandler creates a new notification handler.
func NewHandler(c *httpclient.Client) *Handler {
	return &Handler{client: c}
}

// EventNotification represents an event notification.
type EventNotification struct {
	ID               string         `json:"id" table:"ID"`
	NotificationType string         `json:"notificationType" table:"TYPE"`
	Enabled          bool           `json:"enabled" table:"ENABLED"`
	AppID            string         `json:"appId,omitempty" table:"APP_ID,wide"`
	Owner            string         `json:"owner,omitempty" table:"OWNER,wide"`
	TriggerConfig    map[string]any `json:"triggerConfig,omitempty" table:"-"`
	ActionConfig     map[string]any `json:"actionConfig,omitempty" table:"-"`
}

// EventNotificationList represents a list of event notifications.
type EventNotificationList struct {
	Results []EventNotification `json:"results"`
	Count   int                 `json:"count"`
}

// ResourceNotification represents a resource notification.
type ResourceNotification struct {
	ID               string `json:"id" table:"ID"`
	NotificationType string `json:"notificationType" table:"TYPE"`
	ResourceID       string `json:"resourceId" table:"RESOURCE_ID"`
	AppID            string `json:"appId,omitempty" table:"APP_ID,wide"`
}

// ResourceNotificationList represents a list of resource notifications.
type ResourceNotificationList struct {
	Results []ResourceNotification `json:"results"`
	Count   int                    `json:"count"`
}

// ListEventNotifications lists event notifications.
func (h *Handler) ListEventNotifications(notificationType string) (*EventNotificationList, error) {
	req := h.client.HTTP().R()

	if notificationType != "" {
		req.SetQueryParam("notificationType", notificationType)
	}

	resp, err := req.Get("/platform/notification/v2/event-notifications")
	if err != nil {
		return nil, fmt.Errorf("list event notifications: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("list event notifications: %w", err)
	}

	var result EventNotificationList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("list event notifications: parse response: %w", err)
	}

	return &result, nil
}

// GetEventNotification gets a specific event notification by ID.
func (h *Handler) GetEventNotification(id string) (*EventNotification, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/notification/v2/event-notifications/%s", id))
	if err != nil {
		return nil, fmt.Errorf("get event notification: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get event notification %q: %w", id, err)
	}

	var result EventNotification
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get event notification: parse response: %w", err)
	}

	return &result, nil
}

// CreateEventNotification creates a new event notification.
func (h *Handler) CreateEventNotification(data []byte) (*EventNotification, error) {
	resp, err := h.client.HTTP().R().
		SetBody(data).
		SetHeader("Content-Type", "application/json").
		Post("/platform/notification/v2/event-notifications")
	if err != nil {
		return nil, fmt.Errorf("create event notification: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("create event notification: %w", err)
	}

	var result EventNotification
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("create event notification: parse response: %w", err)
	}

	return &result, nil
}

// DeleteEventNotification deletes an event notification.
func (h *Handler) DeleteEventNotification(id string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/notification/v2/event-notifications/%s", id))
	if err != nil {
		return fmt.Errorf("delete event notification: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("delete event notification %q: %w", id, err)
	}

	return nil
}

// ListResourceNotifications lists resource notifications.
func (h *Handler) ListResourceNotifications(notificationType, resourceID string) (*ResourceNotificationList, error) {
	req := h.client.HTTP().R()

	if notificationType != "" {
		req.SetQueryParam("notificationType", notificationType)
	}
	if resourceID != "" {
		req.SetQueryParam("resourceId", resourceID)
	}

	resp, err := req.Get("/platform/notification/v2/resource-notifications")
	if err != nil {
		return nil, fmt.Errorf("list resource notifications: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("list resource notifications: %w", err)
	}

	var result ResourceNotificationList
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("list resource notifications: parse response: %w", err)
	}

	return &result, nil
}

// GetResourceNotification gets a specific resource notification by ID.
func (h *Handler) GetResourceNotification(id string) (*ResourceNotification, error) {
	resp, err := h.client.HTTP().R().
		Get(fmt.Sprintf("/platform/notification/v2/resource-notifications/%s", id))
	if err != nil {
		return nil, fmt.Errorf("get resource notification: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return nil, fmt.Errorf("get resource notification %q: %w", id, err)
	}

	var result ResourceNotification
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("get resource notification: parse response: %w", err)
	}

	return &result, nil
}

// DeleteResourceNotification deletes a resource notification.
func (h *Handler) DeleteResourceNotification(id string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/notification/v2/resource-notifications/%s", id))
	if err != nil {
		return fmt.Errorf("delete resource notification: %w", err)
	}
	if err := httpclient.CheckResponse(resp); err != nil {
		return fmt.Errorf("delete resource notification %q: %w", id, err)
	}

	return nil
}
