package docs

import (
	"embed"
	"fmt"
	"net/http"
	"strings"
)

//go:embed openapi/*
var openapiFS embed.FS

// RegisterRoutes registers the /docs, /openapi.yaml, and spec file handlers on the mux.
func RegisterRoutes(mux *http.ServeMux, isDataPlane bool) {
	mux.HandleFunc("GET /docs", DocsHandler())
	mux.HandleFunc("GET /docs/", DocsHandler())
	mux.HandleFunc("GET /openapi.yaml", FileHandler("openapi.yaml"))
	mux.HandleFunc("GET /docs/openapi/{file}", DynamicFileHandler())

	if isDataPlane {
		mux.HandleFunc("GET /{$}", RootHandler())
	}
}

// RootHandler returns a JSON service index or redirects browsers to /docs.
func RootHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.Redirect(w, r, "/docs", http.StatusTemporaryRedirect)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
  "service": "llm-gateway",
  "status": "online",
  "documentation": "/docs",
  "openapi_spec": "/openapi.yaml",
  "health": "/health",
  "metrics": "/metrics",
  "data_plane": {
    "chat_completions": "/v1/chat/completions",
    "messages": "/v1/messages",
    "embeddings": "/v1/embeddings",
    "models": "/v1/models"
  }
}`))
	}
}

// FileHandler serves a static embedded file by name.
func FileHandler(fileName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		content, err := openapiFS.ReadFile("openapi/" + fileName)
		if err != nil {
			http.Error(w, fmt.Sprintf("File not found: %s", fileName), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}
}

// DynamicFileHandler serves embedded yaml files matching {file} from openapi/.
func DynamicFileHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fileName := r.PathValue("file")
		if fileName == "" {
			parts := strings.Split(r.URL.Path, "/docs/openapi/")
			if len(parts) > 1 {
				fileName = parts[1]
			}
		}

		cleanName := strings.TrimPrefix(fileName, "/")
		if !strings.HasSuffix(cleanName, ".yaml") && !strings.HasSuffix(cleanName, ".yml") {
			http.Error(w, "Invalid file format", http.StatusBadRequest)
			return
		}

		content, err := openapiFS.ReadFile("openapi/" + cleanName)
		if err != nil {
			http.Error(w, fmt.Sprintf("Spec file '%s' not found", cleanName), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}
}

// DocsHandler renders interactive Swagger UI (or Scalar UI via ?ui=scalar).
func DocsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uiType := r.URL.Query().Get("ui")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		if uiType == "scalar" {
			w.Write([]byte(scalarHTML))
			return
		}

		w.Write([]byte(swaggerHTML))
	}
}

const swaggerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>LLM Gateway - API Reference & Partitions</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
  <link rel="icon" type="image/svg+xml" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>⚡</text></svg>">
  <style>
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #fafafa; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
    .top-banner {
      background: #111827;
      color: #f3f4f6;
      padding: 12px 24px;
      display: flex;
      justify-content: space-between;
      align-items: center;
      box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    }
    .top-banner .brand {
      display: flex;
      align-items: center;
      gap: 10px;
      font-weight: 700;
      font-size: 16px;
      color: #ffffff;
      text-decoration: none;
    }
    .top-banner .links {
      display: flex;
      gap: 16px;
      align-items: center;
      font-size: 14px;
    }
    .top-banner .links a {
      color: #9ca3af;
      text-decoration: none;
      transition: color 0.2s;
    }
    .top-banner .links a:hover {
      color: #60a5fa;
    }
    .badge {
      background: #374151;
      color: #93c5fd;
      padding: 3px 8px;
      border-radius: 6px;
      font-size: 12px;
      font-weight: 600;
    }
    .swagger-ui .topbar { display: none; }
  </style>
</head>
<body>
  <div class="top-banner">
    <a href="/docs" class="brand">
      <span>⚡ LLM Gateway</span>
      <span class="badge">OpenAPI 3.0</span>
    </a>
    <div class="links">
      <a href="/docs?ui=scalar">Switch to Scalar UI</a>
      <a href="/openapi.yaml" target="_blank">Raw openapi.yaml</a>
      <a href="/health" target="_blank">Health Status</a>
      <a href="/metrics" target="_blank">Metrics</a>
    </div>
  </div>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = function() {
      window.ui = SwaggerUIBundle({
        urls: [
          { url: "/openapi.yaml", name: "Unified LLM Gateway API (All Routes)" },
          { url: "/docs/openapi/data-plane.yaml", name: "Data Plane (Inference Proxy - Port 8080)" },
          { url: "/docs/openapi/control-plane.yaml", name: "Control Plane (Management API - Port 8081)" }
        ],
        "urls.primaryName": "Unified LLM Gateway API (All Routes)",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout",
        persistAuthorization: true
      });
    };
  </script>
</body>
</html>
`

const scalarHTML = `<!doctype html>
<html>
  <head>
    <title>LLM Gateway - Scalar API Reference</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style>
      body { margin: 0; }
    </style>
  </head>
  <body>
    <script id="api-reference" data-url="/openapi.yaml" data-proxy-url=""></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>
`
