// tpt-payments/backend/server/server.go
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/redis/go-redis/v9"
	"github.com/signintech/gopdf"
	"github.com/tpt-payments/tpt-payments/backend/analytics"
)

// Server represents the API HTTP server
type Server struct {
	db      *sql.DB
	redis   *redis.Client
	router  *chi.Mux
	server  *http.Server
	analytics *analytics.ReportGenerator
	mcService *multicurrency.Service
}

// NewServer creates a new API server instance
func NewServer(db *sql.DB, redisClient *redis.Client) *Server {
	// Initialize multicurrency service
	mcService := multicurrency.NewService(db, nil) // TODO: Inject actual exchange rate provider
	
	s := &Server{
		db:         db,
		redis:      redisClient,
		router:     chi.NewRouter(),
		analytics:  analytics.NewReportGenerator(db, mcService),
		mcService:  mcService,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

func (s *Server) setupMiddleware() {
	// Standard middleware stack
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(60 * time.Second))
	s.router.Use(middleware.AllowContentType("application/json"))
	s.router.Use(middleware.CleanPath)

	// CORS configuration
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "Idempotency-Key", "Stripe-Version"},
		ExposedHeaders:   []string{"Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Custom middleware
	s.router.Use(s.APIKeyAuth)
	s.router.Use(s.RequestLogging)
	s.router.Use(s.RateLimiter)
}

func (s *Server) setupRoutes() {
	s.router.Get("/health", s.HealthCheck)
	s.router.Get("/openapi.json", s.OpenAPISpec)

	// Public routes (no auth required)
	s.router.Group(func(r chi.Router) {
		// Skip auth middleware for public routes
		r.Use(middleware.AllowContentType("application/json", "text/html", "application/pdf"))
		r.Get("/invoices/{id}", s.PublicGetInvoice)
		r.Get("/invoices/{id}/pdf", s.PublicGetInvoicePDF)
		r.Post("/invoices/{id}/pay", s.PublicPayInvoice)
	})

	// API v1 routes
	s.router.Route("/v1", func(r chi.Router) {
		// Payment Intents
		r.Post("/payment_intents", s.CreatePaymentIntent)
		r.Get("/payment_intents/{id}", s.GetPaymentIntent)
		r.Post("/payment_intents/{id}/confirm", s.ConfirmPaymentIntent)
		r.Post("/payment_intents/{id}/capture", s.CapturePaymentIntent)
		r.Post("/payment_intents/{id}/cancel", s.CancelPaymentIntent)
		r.Get("/payment_intents", s.ListPaymentIntents)

		// Refunds
		r.Post("/refunds", s.CreateRefund)
		r.Get("/refunds/{id}", s.GetRefund)
		r.Get("/refunds", s.ListRefunds)

		// Webhook Endpoints
		r.Post("/webhook_endpoints", s.CreateWebhookEndpoint)
		r.Get("/webhook_endpoints/{id}", s.GetWebhookEndpoint)
		r.Delete("/webhook_endpoints/{id}", s.DeleteWebhookEndpoint)
		r.Post("/webhook_endpoints/{id}/test", s.TestWebhookEndpoint)

		// API Keys
		r.Post("/api_keys", s.CreateAPIKey)
		r.Get("/api_keys", s.ListAPIKeys)
		r.Post("/api_keys/{id}/revoke", s.RevokeAPIKey)
		
		// Analytics
		r.Get("/analytics/revenue", s.GetRevenueReport)
		r.Get("/analytics/success-rate", s.GetSuccessRateReport)
		r.Get("/analytics/refund-rate", s.GetRefundRateReport)
		r.Get("/analytics/subscriptions", s.GetSubscriptionReport)
		r.Get("/analytics/export", s.ExportReport)
	})
}

// Start starts the HTTP server on the specified address
func (s *Server) Start(addr string) error {
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s.server.ListenAndServe()
}

// Shutdown gracefully stops the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// HealthCheck handles health check requests
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// OpenAPISpec returns the OpenAPI 3.0 specification for the API
func (s *Server) OpenAPISpec(w http.ResponseWriter, r *http.Request) {
	spec := map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title":       "TPT Payments API",
			"description": "Payment processing API",
			"version":     "1.0.0",
		},
		"servers": []interface{}{
			map[string]string{
				"url": "https://api.tpt-payments.com/v1",
			},
		},
		"paths": map[string]interface{}{
			"/payment_intents": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Create a Payment Intent",
					"description": "Creates a new Payment Intent object",
					"operationId": "createPaymentIntent",
					"responses": map[string]interface{}{
						"200": map[string]string{"description": "Successful operation"},
					},
				},
				"get": map[string]interface{}{
					"summary":     "List Payment Intents",
					"description": "Returns a list of Payment Intents",
					"operationId": "listPaymentIntents",
					"responses": map[string]interface{}{
						"200": map[string]string{"description": "Successful operation"},
					},
				},
			},
			"/payment_intents/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Retrieve a Payment Intent",
					"description": "Retrieves the details of an existing Payment Intent",
					"operationId": "getPaymentIntent",
					"responses": map[string]interface{}{
						"200": map[string]string{"description": "Successful operation"},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"ApiKeyAuth": map[string]string{
					"type": "apiKey",
					"in":   "header",
					"name": "Authorization",
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(spec)
}

// Placeholder handler implementations (to be completed)
func (s *Server) CreatePaymentIntent(w http.ResponseWriter, r *http.Request) {}
func (s *Server) GetPaymentIntent(w http.ResponseWriter, r *http.Request) {}
func (s *Server) ConfirmPaymentIntent(w http.ResponseWriter, r *http.Request) {}
func (s *Server) CapturePaymentIntent(w http.ResponseWriter, r *http.Request) {}
func (s *Server) CancelPaymentIntent(w http.ResponseWriter, r *http.Request) {}
func (s *Server) ListPaymentIntents(w http.ResponseWriter, r *http.Request) {}

// Analytics handlers
func (s *Server) GetRevenueReport(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement actual revenue report generation
	// For now, return placeholder data
	merchantID := r.URL.Query().Get("merchant_id")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	granularityStr := r.URL.Query().Get("granularity")
	
	// Parse parameters (simplified)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"timeframe":   "2026-05-01 - 2026-05-31",
		"granularity": "daily",
		"data_points": []map[string]interface{}{
			map[string]interface{}{
				"timestamp":  time.Now().Unix(),
				"amount":     "1000.00",
				"volume":     10,
				"currency":   "USD",
			},
		},
		"total_revenue":     "1000.00",
		"total_volume":      10,
		"average_transaction": "100.00",
	})
}

func (s *Server) GetSuccessRateReport(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement actual success rate report generation
	// For now, return placeholder data
	merchantID := r.URL.Query().Get("merchant_id")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	granularityStr := r.URL.Query().Get("granularity")
	
	// Parse parameters (simplified)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"timeframe":        "2026-05-01 - 2026-05-31",
		"granularity":      "daily",
		"data_points":      []map[string]interface{}{
			map[string]interface{}{
				"timestamp":  time.Now().Unix(),
				"success_rate": 0.95,
				"attempts":     20,
				"successes":    19,
			},
		},
		"overall_success_rate": 0.95,
		"total_attempts":       20,
		"total_successes":      19,
	})
}

func (s *Server) GetRefundRateReport(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement actual refund rate report generation
	// For now, return placeholder data
	merchantID := r.URL.Query().Get("merchant_id")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	granularityStr := r.URL.Query().Get("granularity")
	
	// Parse parameters (simplified)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"timeframe":       "2026-05-01 - 2026-05-31",
		"granularity":     "daily",
		"data_points":     []map[string]interface{}{
			map[string]interface{}{
				"timestamp":  time.Now().Unix(),
				"refund_rate": 0.05,
				"refunds":     1,
				"transactions": 20,
			},
		},
		"overall_refund_rate": 0.05,
		"total_refunds":       1,
		"total_transactions":  20,
	})
}

func (s *Server) GetSubscriptionReport(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement actual subscription report generation
	// For now, return placeholder data
	merchantID := r.URL.Query().Get("merchant_id")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	
	// Parse parameters (simplified)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"timeframe":              "2026-05-01 - 2026-05-31",
		"granularity":            "daily",
		"mrr":                    "5000.00",
		"active_subscriptions":   50,
		"churn_rate":             0.02,
		"new_subscriptions":      5,
		"canceled_subscriptions": 1,
	})
}

func (s *Server) ExportReport(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement actual report export
	// For now, return placeholder data
	reportType := r.URL.Query().Get("type")
	format := r.URL.Query().Get("format")
	merchantID := r.URL.Query().Get("merchant_id")
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":  fmt.Sprintf("Exporting %s report in %s format for merchant %s", reportType, format, merchantID),
		"status":   "pending",
		"download_url": "/exports/report-123.pdf",
	})
}

// Refund API Handlers
func (s *Server) CreateRefund(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PaymentIntentID string `json:"payment_intent"`
		Amount          int64  `json:"amount"`
		Currency        string `json:"currency"`
		Reason          string `json:"reason,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"type":    "invalid_request",
				"message": "Invalid request body",
			},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      "re_test",
		"object":  "refund",
		"status":  "pending",
		"amount":  req.Amount,
	})
}

func (s *Server) GetRefund(w http.ResponseWriter, r *http.Request) {
	refundID := chi.URLParam(r, "id")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":     refundID,
		"object": "refund",
		"status": "succeeded",
	})
}

func (s *Server) ListRefunds(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"object":   "list",
		"has_more": false,
		"data":     []interface{}{},
	})
}

func (s *Server) CreateWebhookEndpoint(w http.ResponseWriter, r *http.Request) {}
func (s *Server) GetWebhookEndpoint(w http.ResponseWriter, r *http.Request) {}
func (s *Server) DeleteWebhookEndpoint(w http.ResponseWriter, r *http.Request) {}
func (s *Server) TestWebhookEndpoint(w http.ResponseWriter, r *http.Request) {}

func (s *Server) CreateAPIKey(w http.ResponseWriter, r *http.Request) {}
func (s *Server) ListAPIKeys(w http.ResponseWriter, r *http.Request) {}
func (s *Server) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {}

// Middleware implementations
func (s *Server) APIKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// Public invoice handlers (no authentication required)
func (s *Server) PublicGetInvoice(w http.ResponseWriter, r *http.Request) {
	invoiceID := chi.URLParam(r, "id")
	
	// TODO: Fetch actual invoice from database
	// Return HTML page for human users, JSON for API requests
	
	accept := r.Header.Get("Accept")
	if accept == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": invoiceID,
			"object": "invoice",
			"status": "open",
			"total": 9900,
			"currency": "usd",
			"hosted_invoice_url": "https://api.tpt-payments.com/invoices/" + invoiceID,
		})
		return
	}
	
	// Return hosted HTML payment page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Invoice ` + invoiceID + ` | TPT Payments</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f7fa; padding: 2rem; }
        .container { max-width: 600px; margin: 0 auto; background: white; border-radius: 12px; box-shadow: 0 4px 20px rgba(0,0,0,0.08); overflow: hidden; }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 2rem; text-align: center; }
        .content { padding: 2rem; }
        .invoice-details { margin: 1.5rem 0; }
        .detail-row { display: flex; justify-content: space-between; padding: 0.75rem 0; border-bottom: 1px solid #eaecef; }
        .total { font-size: 1.5rem; font-weight: 700; margin-top: 1rem; padding-top: 1rem; border-top: 2px solid #eaecef; }
        .pay-button { width: 100%; padding: 1rem; background: #667eea; color: white; border: none; border-radius: 8px; font-size: 1.1rem; font-weight: 600; cursor: pointer; margin-top: 2rem; transition: background 0.2s; }
        .pay-button:hover { background: #5a6fd6; }
        .status-badge { display: inline-block; padding: 0.5rem 1rem; border-radius: 20px; font-weight: 600; background: #f0f9ff; color: #0369a1; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Invoice</h1>
            <p>` + invoiceID + `</p>
        </div>
        <div class="content">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem;">
                <span class="status-badge">Open</span>
                <span>Due: ` + time.Now().AddDate(0, 0, 14).Format("January 02, 2006") + `</span>
            </div>
            
            <div class="invoice-details">
                <div class="detail-row">
                    <span>Subscription Renewal</span>
                    <span>$99.00</span>
                </div>
                <div class="detail-row">
                    <span>Tax</span>
                    <span>$0.00</span>
                </div>
                <div class="detail-row total">
                    <span>Total</span>
                    <span>$99.00 USD</span>
                </div>
            </div>
            
            <button class="pay-button" onclick="window.location.href = '/invoices/` + invoiceID + `/pay'">
                Pay Now
            </button>
        </div>
    </div>
</body>
</html>`
	w.Write([]byte(html))
}

func (s *Server) PublicGetInvoicePDF(w http.ResponseWriter, r *http.Request) {
	invoiceID := chi.URLParam(r, "id")
	
	// Initialize PDF generator
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{ PageSize: *gopdf.PageSizeA4 })
	pdf.AddPage()
	
	// Add standard font
	err := pdf.AddTTFFont("Helvetica", "./assets/fonts/Helvetica.ttf")
	if err != nil {
		// Fallback to built-in font
		pdf.SetFont("Courier", "", 12)
	} else {
		pdf.SetFont("Helvetica", "", 12)
	}
	
	// Invoice Header
	pdf.SetXY(40, 40)
	pdf.SetFontSize(24)
	pdf.Cell(nil, "INVOICE")
	
	pdf.SetXY(400, 40)
	pdf.SetFontSize(12)
	pdf.Cell(nil, invoiceID)
	
	pdf.Line(40, 60, 555, 60)
	
	// Invoice Details
	pdf.SetXY(40, 80)
	pdf.SetFontSize(12)
	pdf.Cell(nil, "Date: " + time.Now().Format("January 02, 2006"))
	
	pdf.SetXY(40, 95)
	pdf.Cell(nil, "Due: " + time.Now().AddDate(0, 0, 14).Format("January 02, 2006"))
	
	pdf.SetXY(40, 110)
	pdf.Cell(nil, "Status: Open")
	
	// Bill To section
	pdf.SetXY(40, 140)
	pdf.SetFontSize(14)
	pdf.Cell(nil, "Bill To:")
	pdf.SetFontSize(12)
	pdf.SetXY(40, 155)
	pdf.Cell(nil, "Customer")
	pdf.SetXY(40, 170)
	pdf.Cell(nil, "customer@example.com")
	
	// Line Items Table Header
	pdf.SetXY(40, 210)
	pdf.SetFontSize(12)
	pdf.CellWithOption(&gopdf.Rect{W: 300, H: 20}, "Description", gopdf.CellOption{Border: gopdf.Left | gopdf.Right | gopdf.Top | gopdf.Bottom, Align: gopdf.Left | gopdf.Middle})
	pdf.CellWithOption(&gopdf.Rect{W: 80, H: 20}, "Quantity", gopdf.CellOption{Border: gopdf.Left | gopdf.Right | gopdf.Top | gopdf.Bottom, Align: gopdf.Center | gopdf.Middle})
	pdf.CellWithOption(&gopdf.Rect{W: 80, H: 20}, "Amount", gopdf.CellOption{Border: gopdf.Left | gopdf.Right | gopdf.Top | gopdf.Bottom, Align: gopdf.Right | gopdf.Middle})
	pdf.SetXY(40, 230)
	
	// Line Items
	pdf.CellWithOption(&gopdf.Rect{W: 300, H: 18}, "Subscription Renewal", gopdf.CellOption{Border: gopdf.Left | gopdf.Right, Align: gopdf.Left | gopdf.Middle})
	pdf.CellWithOption(&gopdf.Rect{W: 80, H: 18}, "1", gopdf.CellOption{Border: gopdf.Left | gopdf.Right, Align: gopdf.Center | gopdf.Middle})
	pdf.CellWithOption(&gopdf.Rect{W: 80, H: 18}, "$99.00", gopdf.CellOption{Border: gopdf.Left | gopdf.Right, Align: gopdf.Right | gopdf.Middle})
	pdf.SetXY(40, 248)
	
	// Subtotal
	pdf.CellWithOption(&gopdf.Rect{W: 380, H: 18}, "Subtotal", gopdf.CellOption{Border: gopdf.Left, Align: gopdf.Right | gopdf.Middle})
	pdf.CellWithOption(&gopdf.Rect{W: 80, H: 18}, "$99.00", gopdf.CellOption{Border: gopdf.Left | gopdf.Right, Align: gopdf.Right | gopdf.Middle})
	pdf.SetXY(40, 266)
	
	// Tax
	pdf.CellWithOption(&gopdf.Rect{W: 380, H: 18}, "Tax", gopdf.CellOption{Border: gopdf.Left, Align: gopdf.Right | gopdf.Middle})
	pdf.CellWithOption(&gopdf.Rect{W: 80, H: 18}, "$0.00", gopdf.CellOption{Border: gopdf.Left | gopdf.Right, Align: gopdf.Right | gopdf.Middle})
	pdf.SetXY(40, 284)
	
	// Total
	pdf.SetFontSize(14)
	pdf.CellWithOption(&gopdf.Rect{W: 380, H: 24}, "Total", gopdf.CellOption{Border: gopdf.Left | gopdf.Top, Align: gopdf.Right | gopdf.Middle})
	pdf.CellWithOption(&gopdf.Rect{W: 80, H: 24}, "$99.00", gopdf.CellOption{Border: gopdf.Left | gopdf.Right | gopdf.Top, Align: gopdf.Right | gopdf.Middle})
	
	// Footer
	pdf.SetXY(40, 750)
	pdf.SetFontSize(10)
	pdf.Cell(nil, "TPT Payments | Generated at " + time.Now().Format(time.RFC3339))
	pdf.SetXY(40, 763)
	pdf.Cell(nil, "Pay online: https://api.tpt-payments.com/invoices/" + invoiceID)
	
	// Set headers
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=invoice-"+invoiceID+".pdf")
	
	// Write PDF to response
	pdf.Write(w)
}

func (s *Server) PublicPayInvoice(w http.ResponseWriter, r *http.Request) {
	invoiceID := chi.URLParam(r, "id")
	
	// TODO: Implement actual payment processing
	// Create payment intent, handle payment method, mark invoice as paid
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id": invoiceID,
		"status": "processing",
		"message": "Payment processing initiated",
	})
}

// responseRecorder wraps http.ResponseWriter to capture status code and response body
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		body:           &bytes.Buffer{},
	}
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// RequestLogging implements structured JSON logging with no PII
func (s *Server) RequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := middleware.GetReqID(r.Context())

		// Read request body without consuming it
		var requestBody []byte
		if r.Body != nil {
			requestBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Create response recorder
		rec := newResponseRecorder(w)

		// Process request
		next.ServeHTTP(rec, r)
		duration := time.Since(start)

		// Extract API key ID (not the full key) from context
		apiKeyID, _ := r.Context().Value("api_key_id").(string)

		// Build structured log entry
		logEntry := map[string]interface{}{
			"timestamp":    start.Format(time.RFC3339Nano),
			"request_id":   requestID,
			"method":       r.Method,
			"path":         r.URL.Path,
			"status":       rec.statusCode,
			"duration_ms":  duration.Milliseconds(),
			"user_agent":   r.UserAgent(),
			"remote_ip":    r.RemoteAddr,
			"api_key_id":   apiKeyID,
			"request_size": len(requestBody),
			"response_size": rec.body.Len(),
		}

		// Log as JSON
		jsonBytes, err := json.Marshal(logEntry)
		if err == nil {
			println(string(jsonBytes))
		}
	})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// RateLimiter implements Redis-backed per API key rate limiting
// 100 requests per minute per API key
func (s *Server) RateLimiter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract API key from request context (set by APIKeyAuth middleware)
		apiKey, ok := r.Context().Value("api_key").(string)
		if !ok {
			// No API key found, allow unauthenticated endpoints (like health check)
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()
		now := time.Now()
		window := now.Truncate(time.Minute)
		key := "rate_limit:" + apiKey + ":" + window.Format("200601021504")

		// Increment counter atomically
		count, err := s.redis.Incr(ctx, key).Result()
		if err != nil {
			// Fail open if Redis is unavailable
			next.ServeHTTP(w, r)
			return
		}

		// Set expiration on first request in window
		if count == 1 {
			s.redis.Expire(ctx, key, 61*time.Second) // 1 extra second for safety
		}

		// Set rate limit headers
		w.Header().Set("X-RateLimit-Limit", "100")
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(max(0, 100-int(count))))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(window.Add(time.Minute).Unix(), 10))

		// Check if limit exceeded
		if count > 100 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"type":    "rate_limit_exceeded",
					"message": "Rate limit exceeded. 100 requests per minute allowed.",
				},
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}
