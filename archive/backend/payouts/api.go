// tpt-payments/backend/payouts/api.go
package payouts

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
)

// ScheduleHTTPHandler returns payout schedule API handler
func ScheduleHTTPHandler(scheduleService *ScheduleService) http.Handler {
	r := chi.NewRouter()

	// GET /v1/payouts/schedule - Get current merchant schedule
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		merchantID := ctx.Value("merchant_id").(string)

		schedule, err := scheduleService.GetSchedule(ctx, merchantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(schedule)
	})

	// POST /v1/payouts/schedule - Update merchant schedule
	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		merchantID := ctx.Value("merchant_id").(string)

		var body struct {
			ScheduleType       PayoutScheduleType `json:"schedule_type"`
			Weekday            int                `json:"weekday,omitempty"`
			CustomIntervalDays int                `json:"custom_interval_days,omitempty"`
			CustomWeekdays     []int              `json:"custom_weekdays,omitempty"`
			MinimumAmount      decimal.Decimal    `json:"minimum_amount"`
			Enabled            bool               `json:"enabled"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		schedule := &PayoutSchedule{
			MerchantID:         merchantID,
			ScheduleType:       body.ScheduleType,
			Weekday:            body.Weekday,
			CustomIntervalDays: body.CustomIntervalDays,
			CustomWeekdays:     body.CustomWeekdays,
			MinimumAmount:      body.MinimumAmount,
			Enabled:            body.Enabled,
			CreatedAt:          time.Now().Unix(),
			UpdatedAt:          time.Now().Unix(),
		}

		if err := schedule.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := scheduleService.SetSchedule(ctx, schedule); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Return the calculated next payout date
		nextDate := scheduleService.CalculateNextPayoutDate(schedule)
		response := struct {
			*PayoutSchedule
			NextPayoutDate int64 `json:"next_payout_date"`
		}{
			PayoutSchedule: schedule,
			NextPayoutDate: nextDate,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	return r
}

// ReconciliationHTTPHandler returns payout reconciliation API handler
func ReconciliationHTTPHandler(reconciler *PayoutReconciler) http.Handler {
	r := chi.NewRouter()

	// GET /v1/payouts/reconciliation - Get reconciliation report for a date
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		dateStr := r.URL.Query().Get("date")
		if dateStr == "" {
			dateStr = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		}

		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			http.Error(w, "invalid date format, expected YYYY-MM-DD", http.StatusBadRequest)
			return
		}

		report, err := reconciler.GetReconciliationReport(ctx, date)
		if err != nil {
			// If no report exists, try to generate one on the fly
			report, err = reconciler.ReconcilePayouts(ctx, date)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(report)
	})

	// GET /v1/payouts/reconciliation/unmatched - List unmatched payouts
	r.Get("/unmatched", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		dateStr := r.URL.Query().Get("date")
		if dateStr == "" {
			dateStr = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		}

		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			http.Error(w, "invalid date format, expected YYYY-MM-DD", http.StatusBadRequest)
			return
		}

		entries, err := reconciler.ListUnmatchedPayouts(ctx, date)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	})

	// POST /v1/payouts/reconciliation/manual-match - Manually match an entry
	r.Post("/manual-match", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var body struct {
			EntryID           string `json:"entry_id"`
			BankTransactionID string `json:"bank_transaction_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if body.EntryID == "" || body.BankTransactionID == "" {
			http.Error(w, "entry_id and bank_transaction_id are required", http.StatusBadRequest)
			return
		}

		if err := reconciler.ManualMatch(ctx, body.EntryID, body.BankTransactionID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "matched"})
	})

	return r
}

// CombinedPayoutHTTPHandler mounts all payout-related handlers
func CombinedPayoutHTTPHandler(
	payoutService *Service,
	scheduleService *ScheduleService,
	reconciler *PayoutReconciler,
) http.Handler {
	r := chi.NewRouter()

	// Mount core payout handlers
	r.Mount("/", HTTPHandler(payoutService))

	// Mount schedule handlers
	r.Mount("/schedule", ScheduleHTTPHandler(scheduleService))

	// Mount reconciliation handlers
	r.Mount("/reconciliation", ReconciliationHTTPHandler(reconciler))

	return r
}
