package httpapi

const (
	PublishedAPIPath       = "/api"
	PublishedHealthzPath   = "/healthz"
	PublishedReadyzPath    = "/readyz"
	PublishedWebsocketPath = "/ws"
	PublishedWebhooksPath  = "/webhooks"
)

var publishedRoutePaths = []string{
	PublishedAPIPath,
	PublishedHealthzPath,
	PublishedReadyzPath,
	PublishedWebsocketPath,
	PublishedWebhooksPath,
}

// PublishedRoutePaths returns the backend paths that must be forwarded by the
// published artifact's API service.
func PublishedRoutePaths() []string {
	return append([]string(nil), publishedRoutePaths...)
}
