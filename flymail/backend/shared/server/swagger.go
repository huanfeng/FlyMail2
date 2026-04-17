package server

import (
	"fmt"
	"os"
	"strings"

	"flymail/shared/config"

	"github.com/gin-gonic/gin"
)

// serveSwaggerFiles serves swagger UI files with custom overrides for specific files
func serveSwaggerFiles(c *gin.Context) {
	path := c.Param("path")

	// Handle root path
	if path == "/" || path == "" {
		path = "/index.html"
	}

	// Remove leading slash
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	// Handle specific file overrides
	switch path {
	case "index.html":
		serveCustomSwaggerIndex(c)
		return
	case "swagger-initializer.js":
		serveCustomSwaggerInitializer(c)
		return
	}

	// Serve other files directly
	filePath := "./swagger/" + path
	c.File(filePath)
}

// serveCustomSwaggerIndex serves a customized version of swagger index.html
func serveCustomSwaggerIndex(c *gin.Context) {
	// Read the original file
	content, err := os.ReadFile("./swagger/index.html")
	if err != nil {
		c.String(500, "Failed to read index.html")
		return
	}

	// Replace the title
	modified := strings.Replace(string(content),
		"<title>Swagger UI</title>",
		"<title>FlyMail API Documentation</title>", 1)

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, modified)
}

// serveCustomSwaggerInitializer serves a customized version of swagger-initializer.js
func serveCustomSwaggerInitializer(c *gin.Context) {
	// Read the original file
	content, err := os.ReadFile("./swagger/swagger-initializer.js")
	if err != nil {
		c.String(500, "Failed to read swagger-initializer.js")
		return
	}

	// Replace the default URL
	modified := strings.Replace(string(content),
		`url: "https://petstore.swagger.io/v2/swagger.json"`,
		`url: "/api/v1/openapi.yaml"`, 1)

	// Add additional configuration options
	// Find the SwaggerUIBundle config and add more options
	modified = strings.Replace(modified,
		`layout: "StandaloneLayout"`,
		`layout: "StandaloneLayout",
    validatorUrl: null,
    tryItOutEnabled: true,
    supportedSubmitMethods: ['get', 'put', 'post', 'delete', 'options', 'head', 'patch', 'trace'],
    docExpansion: "list",
    persistAuthorization: true`, 1)

	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.String(200, modified)
}

// serveOpenAPISpec serves the OpenAPI specification with dynamic server URL
func serveOpenAPISpec(config *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Read the OpenAPI spec file
		content, err := os.ReadFile("./api/v1/openapi.yaml")
		if err != nil {
			c.String(500, "Failed to read openapi.yaml")
			return
		}

		// Get the current request's scheme and host
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}

		// Get host from request
		host := c.Request.Host
		if forwardedHost := c.GetHeader("X-Forwarded-Host"); forwardedHost != "" {
			host = forwardedHost
		}

		// Construct the dynamic server URL
		serverURL := fmt.Sprintf("%s://%s/api/v1", scheme, host)

		// Replace the servers section with dynamic URL
		yamlStr := string(content)
		// Find and replace the servers section
		serversStart := strings.Index(yamlStr, "servers:")
		if serversStart != -1 {
			// Find the next section after servers (security: in this case)
			serversEnd := strings.Index(yamlStr[serversStart:], "security:")
			if serversEnd != -1 {
				serversEnd += serversStart
				// Build new servers section
				newServersSection := fmt.Sprintf("servers:\n  - url: %s\n\n", serverURL)
				// Replace the old servers section
				yamlStr = yamlStr[:serversStart] + newServersSection + yamlStr[serversEnd:]
			}
		}

		// Set appropriate headers
		c.Header("Content-Type", "text/vnd.yaml; charset=utf-8")
		c.String(200, yamlStr)
	}
}
