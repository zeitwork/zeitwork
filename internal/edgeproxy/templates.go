package edgeproxy

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
)

//go:embed templates/*.html
var templateFiles embed.FS

// TemplateManager handles error page templates
type TemplateManager struct {
	templates map[int]*template.Template
}

// NewTemplateManager creates a new template manager
func NewTemplateManager() (*TemplateManager, error) {
	tm := &TemplateManager{
		templates: make(map[int]*template.Template),
	}

	// Load common error templates
	errorCodes := []int{400, 401, 403, 404, 500, 502, 503, 504}
	for _, code := range errorCodes {
		tmpl, err := tm.loadTemplate(code)
		if err != nil {
			// If template doesn't exist, create a default one
			tmpl = tm.createDefaultTemplate(code)
		}
		tm.templates[code] = tmpl
	}

	return tm, nil
}

// loadTemplate loads a template from the embedded filesystem
func (tm *TemplateManager) loadTemplate(statusCode int) (*template.Template, error) {
	templatePath := fmt.Sprintf("templates/%d.html", statusCode)
	content, err := templateFiles.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New(fmt.Sprintf("%d", statusCode)).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", templatePath, err)
	}

	return tmpl, nil
}

// createDefaultTemplate creates a default template for an error code
func (tm *TemplateManager) createDefaultTemplate(statusCode int) *template.Template {
	statusText := http.StatusText(statusCode)
	defaultHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%d %s</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            margin: 0;
            padding: 0;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .error-container {
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 40px rgba(0,0,0,0.1);
            padding: 3rem;
            text-align: center;
            max-width: 500px;
            margin: 2rem;
        }
        .error-code {
            font-size: 6rem;
            font-weight: 700;
            color: #667eea;
            margin: 0;
            line-height: 1;
        }
        .error-message {
            font-size: 1.5rem;
            color: #333;
            margin: 1rem 0 2rem;
        }
        .error-description {
            color: #666;
            margin-bottom: 2rem;
            line-height: 1.6;
        }
        .retry-btn {
            background: #667eea;
            color: white;
            border: none;
            padding: 0.75rem 2rem;
            border-radius: 6px;
            font-size: 1rem;
            cursor: pointer;
            transition: background 0.2s;
        }
        .retry-btn:hover {
            background: #5a67d8;
        }
        .timestamp {
            font-size: 0.8rem;
            color: #999;
            margin-top: 2rem;
        }
    </style>
</head>
<body>
    <div class="error-container">
        <h1 class="error-code">%d</h1>
        <h2 class="error-message">%s</h2>
        <p class="error-description">{{.Description}}</p>
        <button class="retry-btn" onclick="window.location.reload()">Try Again</button>
        <div class="timestamp">{{.Timestamp}}</div>
    </div>
</body>
</html>`, statusCode, statusText, statusCode, statusText)

	tmpl, _ := template.New(fmt.Sprintf("%d", statusCode)).Parse(defaultHTML)
	return tmpl
}

// isBrowserRequest checks if the request is from a browser based on User-Agent and Accept headers
func isBrowserRequest(r *http.Request) bool {
	userAgent := strings.ToLower(r.Header.Get("User-Agent"))
	accept := strings.ToLower(r.Header.Get("Accept"))

	// Check for common browser user agents
	browserAgents := []string{"mozilla", "chrome", "safari", "firefox", "edge", "opera"}
	for _, agent := range browserAgents {
		if strings.Contains(userAgent, agent) {
			// Also check if the request accepts HTML
			if strings.Contains(accept, "text/html") {
				return true
			}
		}
	}

	// Check if Accept header explicitly requests HTML
	if strings.Contains(accept, "text/html") {
		return true
	}

	return false
}

// TemplateData holds data for template rendering
type TemplateData struct {
	Domain      string
	StatusCode  int
	StatusText  string
	Description string
	Timestamp   string
}

// ServeErrorPage serves an error page, choosing between HTML template or plain text
func (tm *TemplateManager) ServeErrorPage(w http.ResponseWriter, r *http.Request, statusCode int, domain string, description string) {
	statusText := http.StatusText(statusCode)

	// Set status code
	w.WriteHeader(statusCode)

	// Check if this is a browser request
	if isBrowserRequest(r) {
		// Serve HTML template
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		tmpl, exists := tm.templates[statusCode]
		if !exists {
			// Fallback to default template
			tmpl = tm.createDefaultTemplate(statusCode)
		}

		data := TemplateData{
			Domain:      domain,
			StatusCode:  statusCode,
			StatusText:  statusText,
			Description: description,
			Timestamp:   fmt.Sprintf("Error occurred at %s", time.Now().Format("2006-01-02 15:04:05 UTC")),
		}

		if err := tmpl.Execute(w, data); err != nil {
			// Fallback to plain text if template execution fails
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintf(w, "%d %s - %s", statusCode, statusText, description)
		}
	} else {
		// Serve plain text for non-browser requests (APIs, curl, etc.)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if description != "" {
			fmt.Fprintf(w, "%d %s - %s", statusCode, statusText, description)
		} else {
			fmt.Fprintf(w, "%d %s", statusCode, statusText)
		}
	}
}
