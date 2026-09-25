package manager

import (
	_ "embed"
	"net/http"
)

// Plugin package assets served at the URLs the ai-plugin.json manifest
// references.

//go:embed assets/codebridge-logo.png
var embeddedLogoPNG []byte

//go:embed assets/codebridge-legal.txt
var embeddedLegalTXT []byte

// serveEmbeddedAsset responds with a baked-in asset.
func serveEmbeddedAsset(data []byte, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(data)
	}
}

// EmbeddedLogoPNG exposes the brand logo for the plugin manifest routes.
func EmbeddedLogoPNG() []byte { return embeddedLogoPNG }

// EmbeddedLegalTXT exposes the legal notice for the plugin manifest routes.
func EmbeddedLegalTXT() []byte { return embeddedLegalTXT }

// ServeEmbeddedAsset is the exported handler factory for baked-in assets.
func ServeEmbeddedAsset(data []byte, contentType string) http.HandlerFunc {
	return serveEmbeddedAsset(data, contentType)
}
