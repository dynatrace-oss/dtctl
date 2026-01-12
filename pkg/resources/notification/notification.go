package notification

import (
	"fmt"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// Handler handles notification resources
type Handler struct {
	client *client.Client
}

// NewHandler creates a new notification handler
func NewHandler(c *client.Client) *Handler {
	return &Handler{client: c}
}

// EventNotification represents an event notification
type EventNotification struct {
	ID               string                 `json:"id" table:"ID"`
	NotificationType string                 `json:"notificationType" table:"TYPE"`
	Enabled          bool                   `json:"enabled" table:"ENABLED"`
	AppID            string                 `json:"appId,omitempty" table:"APP_ID,wide"`
	Owner            string                 `json:"owner,omitempty" table:"OWNER,wide"`
	TriggerConfig    map[string]interface{} `json:"triggerConfig,omitempty" table:"-"`
	ActionConfig     map[string]interface{} `json:"actionConfig,omitempty" table:"-"`
}

// EventNotificationList represents a list of event notifications
type EventNotificationList struct {
	Results []EventNotification `json:"results"`
	Count   int                 `json:"count"`
}

// ResourceNotification represents a resource notification
type ResourceNotification struct {
	ID               string `json:"id" table:"ID"`
	NotificationType string `json:"notificationType" table:"TYPE"`
	ResourceID       string `json:"resourceId" table:"RESOURCE_ID"`
	AppID            string `json:"appId,omitempty" table:"APP_ID,wide"`
}

// ResourceNotificationList represents a list of resource notifications
type ResourceNotificationList struct {
	Results []ResourceNotification `json:"results"`
	Count   int                    `json:"count"`
}

// ListEventNotifications lists event notifications
func (h *Handler) ListEventNotifications(notificationType string) (*EventNotificationList, error) {
	var result EventNotificationList

	req := h.client.HTTP().R().SetResult(&result)

	if notificationType != "" {
		req.SetQueryParam("notificationType", notificationType)
	}

	resp, err := req.Get("/platform/notification/v2/event-notifications")

	if err != nil {
		return nil, fmt.Errorf("failed to list event notifications: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list event notifications: status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// GetEventNotification gets a specific event notification by ID
func (h *Handler) GetEventNotification(id string) (*EventNotification, error) {
	var result EventNotification

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/notification/v2/event-notifications/%s", id))

	if err != nil {
		return nil, fmt.Errorf("failed to get event notification: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("event notification %q not found", id)
		default:
			return nil, fmt.Errorf("failed to get event notification: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// CreateEventNotification creates a new event notification
func (h *Handler) CreateEventNotification(data []byte) (*EventNotification, error) {
	var result EventNotification

	resp, err := h.client.HTTP().R().
		SetBody(data).
		SetResult(&result).
		SetHeader("Content-Type", "application/json").
		Post("/platform/notification/v2/event-notifications")

	if err != nil {
		return nil, fmt.Errorf("failed to create event notification: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 400:
			return nil, fmt.Errorf("invalid notification configuration: %s", resp.String())
		case 403:
			return nil, fmt.Errorf("access denied to create notification")
		default:
			return nil, fmt.Errorf("failed to create event notification: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// DeleteEventNotification deletes an event notification
func (h *Handler) DeleteEventNotification(id string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/notification/v2/event-notifications/%s", id))

	if err != nil {
		return fmt.Errorf("failed to delete event notification: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return fmt.Errorf("event notification %q not found", id)
		default:
			return fmt.Errorf("failed to delete event notification: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}

// ListResourceNotifications lists resource notifications
func (h *Handler) ListResourceNotifications(notificationType, resourceID string) (*ResourceNotificationList, error) {
	var result ResourceNotificationList

	req := h.client.HTTP().R().SetResult(&result)

	if notificationType != "" {
		req.SetQueryParam("notificationType", notificationType)
	}
	if resourceID != "" {
		req.SetQueryParam("resourceId", resourceID)
	}

	resp, err := req.Get("/platform/notification/v2/resource-notifications")

	if err != nil {
		return nil, fmt.Errorf("failed to list resource notifications: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("failed to list resource notifications: status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// GetResourceNotification gets a specific resource notification by ID
func (h *Handler) GetResourceNotification(id string) (*ResourceNotification, error) {
	var result ResourceNotification

	resp, err := h.client.HTTP().R().
		SetResult(&result).
		Get(fmt.Sprintf("/platform/notification/v2/resource-notifications/%s", id))

	if err != nil {
		return nil, fmt.Errorf("failed to get resource notification: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return nil, fmt.Errorf("resource notification %q not found", id)
		default:
			return nil, fmt.Errorf("failed to get resource notification: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return &result, nil
}

// DeleteResourceNotification deletes a resource notification
func (h *Handler) DeleteResourceNotification(id string) error {
	resp, err := h.client.HTTP().R().
		Delete(fmt.Sprintf("/platform/notification/v2/resource-notifications/%s", id))

	if err != nil {
		return fmt.Errorf("failed to delete resource notification: %w", err)
	}

	if resp.IsError() {
		switch resp.StatusCode() {
		case 404:
			return fmt.Errorf("resource notification %q not found", id)
		default:
			return fmt.Errorf("failed to delete resource notification: status %d: %s", resp.StatusCode(), resp.String())
		}
	}

	return nil
}
