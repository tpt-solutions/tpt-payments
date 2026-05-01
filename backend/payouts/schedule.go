// tpt-payments/backend/payouts/schedule.go
package payouts

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shopspring/decimal"
)


// PayoutScheduleType represents payout frequency
type PayoutScheduleType string

const (
	PayoutScheduleDaily   PayoutScheduleType = "daily"
	PayoutScheduleWeekly  PayoutScheduleType = "weekly"
	PayoutScheduleCustom  PayoutScheduleType = "custom"
	PayoutScheduleManual  PayoutScheduleType = "manual"
	PayoutScheduleInstant PayoutScheduleType = "instant"
)


// PayoutSchedule represents merchant payout configuration
type PayoutSchedule struct {
	MerchantID         string             `json:"merchant_id"`
	ScheduleType       PayoutScheduleType `json:"schedule_type"`
	Weekday            int                `json:"weekday,omitempty"`            // 0 = Sunday, 1 = Monday, etc. for weekly
	CustomIntervalDays int                `json:"custom_interval_days,omitempty"` // e.g. every 3 days for custom
	CustomWeekdays     []int              `json:"custom_weekdays,omitempty"`      // specific weekdays [1,3,5] for custom
	MinimumAmount      decimal.Decimal    `json:"minimum_amount"`
	Enabled            bool               `json:"enabled"`
	UpdatedAt          int64              `json:"updated_at"`
	CreatedAt          int64              `json:"created_at"`
}


// PayoutQueueItem represents a payout queued for processing
type PayoutQueueItem struct {
	ID         string          `json:"id"`
	MerchantID string          `json:"merchant_id"`
	Amount     decimal.Decimal `json:"amount"`
	Currency   string          `json:"currency"`
	Status     string          `json:"status"`
	ScheduledAt int64         `json:"scheduled_at"`
	ProcessedAt int64         `json:"processed_at,omitempty"`
	CreatedAt  int64           `json:"created_at"`
}

// ScheduleService handles payout scheduling
type ScheduleService struct {
	db *sql.DB
}

// NewScheduleService creates payout schedule service
func NewScheduleService(db *sql.DB) *ScheduleService {
	return &ScheduleService{db: db}
}

// GetSchedule returns payout schedule for a merchant
func (s *ScheduleService) GetSchedule(ctx context.Context, merchantID string) (*PayoutSchedule, error) {
	var schedule PayoutSchedule
	
	err := s.db.QueryRowContext(ctx, `
		SELECT merchant_id, schedule_type, weekday, minimum_amount, enabled, updated_at, created_at
		FROM payout_schedules WHERE merchant_id = $1`,
		merchantID,
	).Scan(
		&schedule.MerchantID, &schedule.ScheduleType, &schedule.Weekday,
		&schedule.MinimumAmount, &schedule.Enabled, &schedule.UpdatedAt, &schedule.CreatedAt,
	)
	
	if err == sql.ErrNoRows {
		// Return default schedule
		return &PayoutSchedule{
			MerchantID:    merchantID,
			ScheduleType:  PayoutScheduleDaily,
			MinimumAmount: decimal.NewFromInt(5000), // $50 minimum
			Enabled:       true,
			CreatedAt:     time.Now().Unix(),
			UpdatedAt:     time.Now().Unix(),
		}, nil
	}
	
	return &schedule, err
}

// SetSchedule updates payout schedule for a merchant
func (s *ScheduleService) SetSchedule(ctx context.Context, schedule *PayoutSchedule) error {
	now := time.Now().Unix()
	schedule.UpdatedAt = now
	
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO payout_schedules (
			merchant_id, schedule_type, weekday, minimum_amount, enabled, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (merchant_id) DO UPDATE SET
			schedule_type = $2, weekday = $3, minimum_amount = $4, enabled = $5, updated_at = $7`,
		schedule.MerchantID, schedule.ScheduleType, schedule.Weekday,
		schedule.MinimumAmount, schedule.Enabled, now, now,
	)
	
	return err
}

// QueuePayout adds a payout to the processing queue
func (s *ScheduleService) QueuePayout(ctx context.Context, merchantID string, amount decimal.Decimal, currency string, scheduledAt int64) error {
	now := time.Now().Unix()
	
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO payout_queue (
			merchant_id, amount, currency, status, scheduled_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		merchantID, amount, currency, "pending", scheduledAt, now,
	)
	
	return err
}

// GetDuePayouts returns payouts ready for processing
func (s *ScheduleService) GetDuePayouts(ctx context.Context) ([]PayoutQueueItem, error) {
	now := time.Now().Unix()
	
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, merchant_id, amount, currency, status, scheduled_at, created_at
		FROM payout_queue WHERE status = 'pending' AND scheduled_at <= $1`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var items []PayoutQueueItem
	for rows.Next() {
		var item PayoutQueueItem
		err := rows.Scan(
			&item.ID, &item.MerchantID, &item.Amount, &item.Currency,
			&item.Status, &item.ScheduledAt, &item.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	
	return items, nil
}

// Validate checks schedule configuration validity
func (s *PayoutSchedule) Validate() error {
	if s.ScheduleType == PayoutScheduleCustom {
		if s.CustomIntervalDays > 0 && len(s.CustomWeekdays) > 0 {
			return errors.New("custom schedule cannot specify both interval_days and weekdays")
		}
		if s.CustomIntervalDays <= 0 && len(s.CustomWeekdays) == 0 {
			return errors.New("custom schedule must specify interval_days or weekdays")
		}
		for _, wd := range s.CustomWeekdays {
			if wd < 0 || wd > 6 {
				return errors.New("custom weekdays must be between 0 (Sunday) and 6 (Saturday)")
			}
		}
	}
	return nil
}

// CalculateNextPayoutDate calculates next payout date based on schedule
func (s *ScheduleService) CalculateNextPayoutDate(schedule *PayoutSchedule) int64 {
	now := time.Now().UTC()

	switch schedule.ScheduleType {
	case PayoutScheduleDaily:
		return now.Add(24 * time.Hour).Truncate(24 * time.Hour).Unix()

	case PayoutScheduleWeekly:
		daysUntil := (schedule.Weekday - int(now.Weekday()) + 7) % 7
		if daysUntil == 0 {
			daysUntil = 7
		}
		return now.AddDate(0, 0, daysUntil).Truncate(24 * time.Hour).Unix()

	case PayoutScheduleCustom:
		if schedule.CustomIntervalDays > 0 {
			// Interval-based: every N days from last payout
			return now.AddDate(0, 0, schedule.CustomIntervalDays).Truncate(24 * time.Hour).Unix()
		}
		if len(schedule.CustomWeekdays) > 0 {
			// Find next matching weekday
			today := int(now.Weekday())
			daysUntil := 7
			for _, wd := range schedule.CustomWeekdays {
				diff := (wd - today + 7) % 7
				if diff == 0 {
					diff = 7 // if today matches, next occurrence is next week
				}
				if diff < daysUntil {
					daysUntil = diff
				}
			}
			return now.AddDate(0, 0, daysUntil).Truncate(24 * time.Hour).Unix()
		}
		return 0

	case PayoutScheduleInstant:
		return now.Unix()

	default: // Manual
		return 0
	}
}
