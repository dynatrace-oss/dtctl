package profile

import (
	"context"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	sdkcla "github.com/dynatrace-oss/dtctl/sdk/api/codelevelanalysis"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// Kind maps user-facing shorthand to API kind strings.
var Kind = sdkcla.Kind

// Payload is the analysis request; re-exported from the SDK for CLI use.
type Payload = sdkcla.Payload

// Response is the analysis result; re-exported from the SDK for CLI use.
type Response = sdkcla.Response

type Handler struct {
	sdk *sdkcla.Handler
}

func NewHandler(c *client.Client) *Handler {
	return &Handler{sdk: sdkcla.NewHandler(httpclient.Wrap(c.HTTP()))}
}

func (h *Handler) Run(ctx context.Context, p Payload) (*Response, error) {
	return h.sdk.Run(ctx, p)
}
