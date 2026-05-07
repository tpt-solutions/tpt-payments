// tpt-payments/backend/internal/reports/scheduled_reports.go
package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/streadway/amqp"
)

// ScheduleFrequency represents how often a report should be sent
type ScheduleFrequency string

const (
	ScheduleFrequencyDaily   ScheduleFrequency = "daily"
	ScheduleFrequencyWeekly  ScheduleFrequency = "weekly"
	ScheduleFrequencyMonthly ScheduleFrequency = "monthly"
)

// ReportFormat represents the output format of the report
type ReportFormat string

const (
	ReportFormatCSV  ReportFormat = "csv"
	ReportFormatJSON ReportFormat = "json"
	ReportFormatPDF  ReportFormat = "pdf"
)

// ScheduledReport represents a merchant's report subscription
type ScheduledReport struct {
	ID         string           `json:"id"`
	MerchantID string           `json:"merchant_id"`
	Name       string           `json:"name"`
	Frequency  ScheduleFrequency `json:"frequency"`
	Format     ReportFormat     `json:"format"`
	Recipients []string         `json:"recipients"`
	Config     ReportConfig    `json:"config"`
	Enabled    bool             `json:"enabled"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	NextRunAt  time.Time        `json:"next_run_at"`
}

// ReportConfig holds the configuration for what data to include in the report
type ReportConfig struct {
	DateRange    string   `json:"date_range"`     // e.g., "last_7_days", "last_30_days"
	IncludeTransactions bool `json:"include_transactions"`
	IncludeRefunds      bool `json:"include_refunds"`
	IncludeSubscriptions bool `json:"include_subscriptions"`
	IncludePayouts      bool `json:"include_payouts"`
	Filters             map[string]string `json:"filters,omitempty"`
}

// ScheduledReportService manages scheduled report creation and delivery
type ScheduledReportService struct {
	repo     ScheduledReportRepository
	emailSvc EmailService
	reportGen ReportGenerator
	queue     *amqp.Channel
}

// ScheduledReportRepository handles persistence of scheduled report configs
type ScheduledReportRepository interface {
	Create(ctx context.Context, report *ScheduledReport) error
	Update(ctx context.Context, report *ScheduledReport) error
	Delete(ctx context.Context, id string) error
	ListByMerchant(ctx context.Context, merchantID string) ([]*ScheduledReport, error)
	ListDue(ctx context.Context, before time.Time) ([]*ScheduledReport, error)
	UpdateNextRun(ctx context.Context, id string, nextRun time.Time) error
}

// EmailService handles sending emails with report attachments
type EmailService interface {
	SendReportEmail(ctx context.Context, to []string, subject string, reportData []byte, format ReportFormat) error
}

// ReportGenerator generates report data in the requested format
type ReportGenerator interface {
	Generate(ctx context.Context, config ReportConfig, format ReportFormat) ([]byte, error)
}

// NewScheduledReportService creates a new scheduled report service
func NewScheduledReportService(
	repo ScheduledReportRepository,
	emailSvc EmailService,
	reportGen ReportGenerator,
	queue *amqp.Channel,
) *ScheduledReportService {
	return &ScheduledReportService{
		repo:     repo,
		emailSvc: emailSvc,
		reportGen: reportGen,
		queue:     queue,
	}
}

// CreateSchedule creates a new scheduled report configuration
func (s *ScheduledReportService) CreateSchedule(ctx context.Context, merchantID, name string, freq ScheduleFrequency, format ReportFormat, recipients []string, config ReportConfig) (*ScheduledReport, error) {
	now := time.Now().UTC()
	report := &ScheduledReport{
		ID:         uuid.New().String(),
		MerchantID: merchantID,
		Name:       name,
		Frequency:  freq,
		Format:     format,
		Recipients: recipients,
		Config:     config,
		Enabled:    true,
		CreatedAt:  now,
		UpdatedAt:  now,
		NextRunAt:  calculateNextRun(now, freq),
	}

	if err := s.repo.Create(ctx, report); err != nil {
		return nil, fmt.Errorf("failed to create scheduled report: %w", err)
	}

	// Enqueue the next run
	if err := s.enqueueNextRun(ctx, report); err != nil {
		// Log but don't fail - the report is created, just scheduling failed
		fmt.Printf("warning: failed to enqueue report schedule: %v\n", err)
	}

	return report, nil
}

// ProcessDueReports processes all reports that are due to run
func (s *ScheduledReportService) ProcessDueReports(ctx context.Context) error {
	now := time.Now().UTC()
	reports, err := s.repo.ListDue(ctx, now)
	if err != nil {
		return fmt.Errorf("failed to list due reports: %w", err)
	}

	for _, report := range reports {
		if !report.Enabled {
			continue
		}

		if err := s.runReport(ctx, report); err != nil {
			fmt.Printf("error running report %s: %v\n", report.ID, err)
			continue
		}

		// Update next run time
		nextRun := calculateNextRun(now, report.Frequency)
		if err := s.repo.UpdateNextRun(ctx, report.ID, nextRun); err != nil {
			fmt.Printf("error updating next run for report %s: %v\n", report.ID, err)
			continue
		}

		// Enqueue next run
		report.NextRunAt = nextRun
		if err := s.enqueueNextRun(ctx, report); err != nil {
			fmt.Printf("warning: failed to enqueue next run for report %s: %v\n", report.ID, err)
		}
	}

	return nil
}

// runReport generates and sends a report
func (s *ScheduledReportService) runReport(ctx context.Context, report *ScheduledReport) error {
	// Generate report data
	reportData, err := s.reportGen.Generate(ctx, report.Config, report.Format)
	if err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	// Send email
	subject := fmt.Sprintf("Scheduled Report: %s - %s", report.Name, time.Now().Format("2006-01-02"))
	if err := s.emailSvc.SendReportEmail(ctx, report.Recipients, subject, reportData, report.Format); err != nil {
		return fmt.Errorf("failed to send report email: %w", err)
	}

	return nil
}

// enqueueNextRun enqueues the next run of a scheduled report using a message queue
func (s *ScheduledReportService) enqueueNextRun(ctx context.Context, report *ScheduledReport) error {
	if s.queue == nil {
		return nil // No queue configured
	}

	message := map[string]interface{}{
		"report_id": report.ID,
		"run_at":    report.NextRunAt.Format(time.RFC3339),
	}

	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	err = s.queue.Publish(
		"scheduled_reports",
		"",
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			DeliveryMode: amqp.Persistent,
			Expiration:  fmt.Sprintf("%d", report.NextRunAt.UnixNano()/1e6),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

// calculateNextRun calculates the next run time based on frequency
func calculateNextRun(from time.Time, freq ScheduleFrequency) time.Time {
	switch freq {
	case ScheduleFrequencyDaily:
		return from.Add(24 * time.Hour)
	case ScheduleFrequencyWeekly:
		return from.Add(7 * 24 * time.Hour)
	case ScheduleFrequencyMonthly:
		return from.AddDate(0, 1, 0)
	default:
		return from.Add(24 * time.Hour)
	}
}