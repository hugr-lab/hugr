package main

import (
	"net/http"
	"slices"
	"strings"
)

type CorsConfig struct {
	CorsAllowedOrigins []string
	CorsAllowedHeaders []string
	CorsAllowedMethods []string
}

func corsMiddleware(c CorsConfig) func(next http.Handler) http.Handler {
	if len(c.CorsAllowedOrigins) == 0 {
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	if len(c.CorsAllowedHeaders) == 0 {
		c.CorsAllowedHeaders = []string{
			"Content-Type", "Authorization", "x-api-key",
			"Accept", "Content-Length", "Accept-Encoding",
			"X-CSRF-Token",
		}
	}
	if len(c.CorsAllowedMethods) == 0 {
		c.CorsAllowedMethods = []string{
			"GET", "POST", "PUT", "DELETE", "OPTIONS",
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Special case: allow '*'
			if len(c.CorsAllowedOrigins) == 1 && c.CorsAllowedOrigins[0] == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Headers", "*")
				w.Header().Set("Access-Control-Allow-Methods", "*")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			origin := r.Header.Get("Origin")
			if origin != "" && slices.Contains(c.CorsAllowedOrigins, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(c.CorsAllowedHeaders, ", "))
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(c.CorsAllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
