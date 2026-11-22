// tpt-payments/backend/internal/compliance/aml_screening.go
package compliance

import (
	"context"
	"fmt"
	"time"
)

// AMLScreeningService handles Anti-Money Laundering screening
type AMLScreeningService struct {
	watchlistProvider WatchlistProvider
	store            AMLStore
	alertService     AlertService
}

// WatchlistProvider interface for OFAC and other watchlists
type WatchlistProvider interface {
	CheckName(ctx context.Context, name string) (*WatchlistMatch, error)
	CheckAddress(ctx context.Context, address string) (*WatchlistMatch, error)
	CheckID(ctx context.Context, idNumber string, idType string) (*WatchlistMatch, error)
	FetchWatchlist(ctx context.Context) error // Update local watchlist cache
}

// WatchlistMatch represents a match against a watchlist
type WatchlistMatch struct {
	ID          string    `json:"id"`
	ListType    string    `json:"list_type"` // OFAC, UN, EU, etc.
	MatchType   string    `json:"match_type"` // name, address, id
	MatchedName string    `json:"matched_name"`
	EntityID    string    `json:"entity_id"` // ID on watchlist
	Score       float64   `json:"score"` // confidence score 0-100
	Details     string    `json:"details"`
	MatchedAt   time.Time `json:"matched_at"`
}

// AMLStore interface for AML screening persistence
type AMLStore interface {
	SaveScreeningResult(ctx context.Context, result *ScreeningResult) error
	GetScreeningResult(ctx context.Context, id string) (*ScreeningResult, error)
	ListScreeningResults(ctx context.Context, filters AMLFilters) ([]*ScreeningResult, error)
	SaveWatchlistMatch(ctx context.Context, match *WatchlistMatch) error
}

// ScreeningResult represents an AML screening result
type ScreeningResult struct {
	ID            string            `json:"id"`
	MerchantID    string            `json:"merchant_id"`
	CustomerID    string            `json:"customer_id,omitempty"`
	EntityType    string            `json:"entity_type"` // merchant, customer
	Status        string            `json:"status"` // cleared, flagged, review_required
	RiskLevel     string            `json:"risk_level"` // low, medium, high
	Matches       []WatchlistMatch  `json:"matches"`
	ScreenedAt    time.Time         `json:"screened_at"`
	ReviewedAt    *time.Time        `json:"reviewed_at,omitempty"`
	ReviewedBy    string            `json:"reviewed_by,omitempty"`
	Notes         string            `json:"notes,omitempty"`
}

// AMLFilters for querying screening results
type AMLFilters struct {
	MerchantID string
	CustomerID string
	Status     string
	RiskLevel  string
	StartDate  *time.Time
	EndDate    *time.Time
}

// NewAMLScreeningService creates a new AML screening service
func NewAMLScreeningService(provider WatchlistProvider, store AMLStore, alertService AlertService) *AMLScreeningService {
	return &AMLScreeningService{
		watchlistProvider: provider,
		store:            store,
		alertService:     alertService,
	}
}

// ScreenMerchant screens a merchant against watchlists
func (s *AMLScreeningService) ScreenMerchant(ctx context.Context, merchantID, name, address string) (*ScreeningResult, error) {
	result := &ScreeningResult{
		ID:         fmt.Sprintf("aml_%s_%d", merchantID, time.Now().Unix()),
		MerchantID: merchantID,
		EntityType: "merchant",
		Status:     "cleared",
		RiskLevel:  "low",
		ScreenedAt: time.Now().UTC(),
		Matches:    []WatchlistMatch{},
	}

	// Check name
	nameMatch, err := s.watchlistProvider.CheckName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("name screening failed: %w", err)
	}
	if nameMatch != nil {
		result.Matches = append(result.Matches, *nameMatch)
	}

	// Check address
	addressMatch, err := s.watchlistProvider.CheckAddress(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("address screening failed: %w", err)
	}
	if addressMatch != nil {
		result.Matches = append(result.Matches, *addressMatch)
	}

	// Determine status and risk level
	if len(result.Matches) > 0 {
		result.Status = "flagged"
		result.RiskLevel = s.calculateRiskLevel(result.Matches)
	}

	// Save result
	if err := s.store.SaveScreeningResult(ctx, result); err != nil {
		return nil, fmt.Errorf("failed to save screening result: %w", err)
	}

	// Alert if flagged
	if result.Status == "flagged" {
		alert := &Alert{
			ID:        fmt.Sprintf("alert_aml_%s", result.ID),
			Type:      "aml_flagged",
			Severity:  "high",
			Message:   fmt.Sprintf("AML screening flagged merchant %s: %s", merchantID, result.RiskLevel),
			MerchantID: merchantID,
			CreatedAt: time.Now().UTC(),
		}
		if err := s.alertService.SendAlert(ctx, alert); err != nil {
			fmt.Printf("Failed to send alert: %v\n", err)
		}
	}

	return result, nil
}

// ScreenCustomer screens a customer against watchlists
func (s *AMLScreeningService) ScreenCustomer(ctx context.Context, customerID, name, address, idNumber, idType string) (*ScreeningResult, error) {
	result := &ScreeningResult{
		ID:         fmt.Sprintf("aml_%s_%d", customerID, time.Now().Unix()),
		CustomerID: customerID,
		EntityType: "customer",
		Status:     "cleared",
		RiskLevel:  "low",
		ScreenedAt: time.Now().UTC(),
		Matches:    []WatchlistMatch{},
	}

	// Check name
	nameMatch, err := s.watchlistProvider.CheckName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("name screening failed: %w", err)
	}
	if nameMatch != nil {
		result.Matches = append(result.Matches, *nameMatch)
	}

	// Check address
	if address != "" {
		addressMatch, err := s.watchlistProvider.CheckAddress(ctx, address)
		if err != nil {
			return nil, fmt.Errorf("address screening failed: %w", err)
		}
		if addressMatch != nil {
			result.Matches = append(result.Matches, *addressMatch)
		}
	}

	// Check ID if provided
	if idNumber != "" && idType != "" {
		idMatch, err := s.watchlistProvider.CheckID(ctx, idNumber, idType)
		if err != nil {
			return nil, fmt.Errorf("ID screening failed: %w", err)
		}
		if idMatch != nil {
			result.Matches = append(result.Matches, *idMatch)
		}
	}

	// Determine status and risk level
	if len(result.Matches) > 0 {
		result.Status = "flagged"
		result.RiskLevel = s.calculateRiskLevel(result.Matches)
	}

	// Save result
	if err := s.store.SaveScreeningResult(ctx, result); err != nil {
		return nil, fmt.Errorf("failed to save screening result: %w", err)
	}

	// Alert if flagged
	if result.Status == "flagged" {
		alert := &Alert{
			ID:        fmt.Sprintf("alert_aml_%s", result.ID),
			Type:      "aml_flagged",
			Severity:  "high",
			Message:   fmt.Sprintf("AML screening flagged customer %s: %s", customerID, result.RiskLevel),
			CreatedAt: time.Now().UTC(),
		}
		if err := s.alertService.SendAlert(ctx, alert); err != nil {
			fmt.Printf("Failed to send alert: %v\n", err)
		}
	}

	return result, nil
}

// calculateRiskLevel determines risk level based on matches
func (s *AMLScreeningService) calculateRiskLevel(matches []WatchlistMatch) string {
	// Simple logic: if any match score > 80, high risk; > 50 medium; else low
	maxScore := 0.0
	for _, match := range matches {
		if match.Score > maxScore {
			maxScore = match.Score
		}
	}

	if maxScore >= 80 {
		return "high"
	} else if maxScore >= 50 {
		return "medium"
	}
	return "low"
}

// ReviewScreeningResult allows manual review of a flagged result
func (s *AMLScreeningService) ReviewScreeningResult(ctx context.Context, resultID, reviewedBy, notes string, approve bool) error {
	result, err := s.store.GetScreeningResult(ctx, resultID)
	if err != nil {
		return fmt.Errorf("failed to get screening result: %w", err)
	}

	now := time.Now().UTC()
	result.ReviewedAt = &now
	result.ReviewedBy = reviewedBy
	result.Notes = notes

	if approve {
		result.Status = "cleared"
	} else {
		result.Status = "review_required"
	}

	return s.store.SaveScreeningResult(ctx, result)
}

// GetAMLSummary returns a summary of AML screening results
func (s *AMLScreeningService) GetAMLSummary(ctx context.Context, filters AMLFilters) (map[string]interface{}, error) {
	results, err := s.store.ListScreeningResults(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list screening results: %w", err)
	}

	summary := map[string]interface{}{
		"total_screened": len(results),
		"by_status":     make(map[string]int),
		"by_risk":      make(map[string]int),
		"by_list_type":  make(map[string]int),
	}

	byStatus := summary["by_status"].(map[string]int)
	byRisk := summary["by_risk"].(map[string]int)
	byListType := summary["by_list_type"].(map[string]int)

	for _, result := range results {
		byStatus[result.Status]++
		byRisk[result.RiskLevel]++

		for _, match := range result.Matches {
			byListType[match.ListType]++
		}
	}

	return summary, nil
}

// UpdateWatchlistCache updates the local watchlist cache
func (s *AMLScreeningService) UpdateWatchlistCache(ctx context.Context) error {
	return s.watchlistProvider.FetchWatchlist(ctx)
}