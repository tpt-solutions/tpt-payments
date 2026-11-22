package payouts

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// =============================================================================
// Bank Account Types
// =============================================================================

// BankAccount represents external bank account details for payouts
type BankAccount struct {
	ID            string `json:"id"`
	MerchantID    string `json:"merchant_id"`
	AccountType   string `json:"account_type"`   // checking, savings
	RoutingNumber string `json:"routing_number"` // ACH routing number
	AccountNumber string `json:"account_number"`
	IBAN          string `json:"iban"`           // SEPA IBAN
	BIC           string `json:"bic"`            // SEPA BIC/SWIFT
	SortCode      string `json:"sort_code"`      // UK Faster Payments
	BankName      string `json:"bank_name"`
	Country       string `json:"country"`
	Currency      string `json:"currency"`
}

// BankAccountStore interface for looking up bank account details
type BankAccountStore interface {
	GetBankAccount(ctx context.Context, accountID string) (*BankAccount, error)
}

// =============================================================================
// ACH Payout Processor (NACHA File Generation + Sponsor Bank)
// =============================================================================

// ACHConfig holds configuration for ACH processing
type ACHConfig struct {
	ODFIIdentification string // Originating Depository Financial Institution
	ODFIRoutingNumber  string // ODFI routing number
	CompanyName        string // Company name for NACHA file
	CompanyID          string // Company tax ID or DUNs
	CompanyEntryDesc   string // Standard Entry Class description
	ImmediateDest      string // Immediate destination routing number
	ImmediateOrigin    string // Immediate origin routing number
	DestinationName    string // Destination bank name
	OriginName         string // Originating bank name
	SponsorBankAPIURL  string // Sponsor bank API endpoint
	SponsorBankAPIKey  string // Sponsor bank API key
	UseFileUpload      bool   // true = generate NACHA file, false = use sponsor bank API
}

// ACHPayoutProcessor handles ACH payouts via NACHA file or sponsor bank API
type ACHPayoutProcessor struct {
	config ACHConfig
	client *http.Client
	store  BankAccountStore
	db     *sql.DB
}

// NewACHPayoutProcessor creates a new ACH processor
func NewACHPayoutProcessor(config ACHConfig, store BankAccountStore) *ACHPayoutProcessor {
	return &ACHPayoutProcessor{
		config: config,
		store:  store,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
	}
}

// WithDB adds database connection for status tracking
func (p *ACHPayoutProcessor) WithDB(db *sql.DB) *ACHPayoutProcessor {
	p.db = db
	return p
}

// InitiatePayout initiates an ACH payout
func (p *ACHPayoutProcessor) InitiatePayout(ctx context.Context, payout *Payout) (string, error) {
	account, err := p.store.GetBankAccount(ctx, payout.DestinationID)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve bank account: %w", err)
	}

	if account.Currency != "usd" {
		return "", fmt.Errorf("ACH only supports USD payouts, got %s", account.Currency)
	}

	if err := validateACHRoutingNumber(account.RoutingNumber); err != nil {
		return "", fmt.Errorf("invalid routing number: %w", err)
	}

	if p.config.UseFileUpload {
		fileRef, err := p.generateNACHAFile(ctx, payout, account)
		if err != nil {
			return "", err
		}
		return fileRef, nil
	}

	return p.initiateViaAPI(ctx, payout, account)
}

// CancelPayout cancels an ACH payout if still pending
func (p *ACHPayoutProcessor) CancelPayout(ctx context.Context, payoutID string) error {
	if !p.config.UseFileUpload {
		req, err := http.NewRequestWithContext(ctx, "POST",
			p.config.SponsorBankAPIURL+"/ach/transfers/"+payoutID+"/cancel", nil)
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "Bearer "+p.config.SponsorBankAPIKey)

		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to cancel ACH transfer: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("sponsor bank cancel error: %s", string(body))
		}
		return nil
	}

	if p.db != nil {
		var fileStatus string
		err := p.db.QueryRowContext(ctx,
			"SELECT status FROM ach_file_tracking WHERE payout_id = $1", payoutID).Scan(&fileStatus)
		if err == nil && fileStatus == "pending" {
			_, err = p.db.ExecContext(ctx,
				"UPDATE ach_file_tracking SET status = 'canceled' WHERE payout_id = $1", payoutID)
			return err
		}
	}

	return fmt.Errorf("ACH file already submitted to bank, cannot cancel")
}

// GetPayoutStatus returns the current status from sponsor bank or file tracking
func (p *ACHPayoutProcessor) GetPayoutStatus(ctx context.Context, payoutID string) (PayoutStatus, error) {
	if p.config.UseFileUpload && p.db != nil {
		var fileStatus string
		err := p.db.QueryRowContext(ctx,
			"SELECT status FROM ach_file_tracking WHERE payout_id = $1", payoutID).Scan(&fileStatus)
		if err == nil {
			switch fileStatus {
			case "pending":
				return PayoutStatusPending, nil
			case "submitted":
				return PayoutStatusInTransit, nil
			case "settled":
				return PayoutStatusPaid, nil
			case "returned", "failed":
				return PayoutStatusFailed, nil
			case "canceled":
				return PayoutStatusCanceled, nil
			}
		}
	}

	if !p.config.UseFileUpload {
		req, err := http.NewRequestWithContext(ctx, "GET",
			p.config.SponsorBankAPIURL+"/ach/transfers/"+payoutID, nil)
		if err != nil {
			return PayoutStatusFailed, err
		}

		req.Header.Set("Authorization", "Bearer "+p.config.SponsorBankAPIKey)

		resp, err := p.client.Do(req)
		if err != nil {
			return PayoutStatusFailed, err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return PayoutStatusFailed, fmt.Errorf("failed to get payout status")
		}

		var result struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return PayoutStatusFailed, err
		}

		switch result.Status {
		case "processed", "settled", "completed":
			return PayoutStatusPaid, nil
		case "pending", "processing":
			return PayoutStatusInTransit, nil
		case "returned", "failed", "rejected":
			return PayoutStatusFailed, nil
		case "canceled":
			return PayoutStatusCanceled, nil
		default:
			return PayoutStatusInTransit, nil
		}
	}

	return PayoutStatusInTransit, nil
}

func (p *ACHPayoutProcessor) generateNACHAFile(ctx context.Context, payout *Payout, account *BankAccount) (string, error) {
	now := time.Now().UTC()
	fileID := fmt.Sprintf("ACH%s%d", payout.ID, now.Unix())

	fileHeader := p.buildFileHeader(now)
	batchHeader := p.buildBatchHeader(now)
	entryDetail := p.buildEntryDetail(payout, account, now)
	batchControl := p.buildBatchControl(payout, now)
	fileControl := p.buildFileControl(payout, now)

	var buf bytes.Buffer
	buf.WriteString(fileHeader + "\n")
	buf.WriteString(batchHeader + "\n")
	buf.WriteString(entryDetail + "\n")
	buf.WriteString(batchControl + "\n")
	buf.WriteString(fileControl + "\n")

	return fileID, nil
}

func (p *ACHPayoutProcessor) buildFileHeader(now time.Time) string {
	return fmt.Sprintf("1%02d%-10s%-10s%-6s%-4s%-3s%-3s%-8s%-9s%-8s%-39s",
		01,
		padRight(p.config.ImmediateDest, 10),
		padRight(p.config.ImmediateOrigin, 10),
		now.Format("060102"),
		now.Format("1504"),
		"A",
		"094",
		padRight(p.config.ODFIIdentification, 8),
		"",
		"1",
		padRight(p.config.DestinationName, 23),
	)
}

func (p *ACHPayoutProcessor) buildBatchHeader(now time.Time) string {
	return fmt.Sprintf("5%3s%-16s%-20s%-10s%-10s%-6s%-3s%-10s%-19s%-6s%-3s%-3s%-8s%-7s",
		"PPD",
		padRight(p.config.CompanyName, 16),
		padRight("PAYROLL", 20),
		padRight(p.config.CompanyID, 10),
		"",
		"",
		"125",
		"",
		"",
		now.Format("060102"),
		"",
		"1",
		"12345678",
		"0000001",
	)
}

func (p *ACHPayoutProcessor) buildEntryDetail(payout *Payout, account *BankAccount, now time.Time) string {
	amountCents := payout.Amount.Mul(decimal.NewFromInt(100)).IntPart()
	return fmt.Sprintf("6%2s%-8s%-1s%-17s%10d%-15s%-22s%-2s%-8s%-7s",
		"22",
		account.RoutingNumber[:8],
		string(account.RoutingNumber[8]),
		padRight(account.AccountNumber, 17),
		amountCents,
		padRight(payout.ID, 15),
		padRight(payout.MerchantID, 22),
		"  ",
		"",
		"0000001",
	)
}

func (p *ACHPayoutProcessor) buildBatchControl(payout *Payout, now time.Time) string {
	amountCents := payout.Amount.Mul(decimal.NewFromInt(100)).IntPart()
	return fmt.Sprintf("8%3s%1s%6d%10d%-12s%-19s%-8s%-7s%-39s",
		"PPD",
		"0",
		1,
		amountCents,
		padRight(p.config.CompanyID, 12),
		"",
		padRight(p.config.ODFIIdentification, 8),
		"0000001",
		"",
	)
}

func (p *ACHPayoutProcessor) buildFileControl(payout *Payout, now time.Time) string {
	amountCents := payout.Amount.Mul(decimal.NewFromInt(100)).IntPart()
	return fmt.Sprintf("9%6d%10d%8d%1s%-39s%-39s",
		1,
		1,
		1,
		"",
		padRight(strconv.FormatInt(amountCents, 10), 39),
		"",
	)
}

func (p *ACHPayoutProcessor) initiateViaAPI(ctx context.Context, payout *Payout, account *BankAccount) (string, error) {
	reqBody := map[string]interface{}{
		"amount":         payout.Amount.String(),
		"currency":       payout.Currency,
		"account_number": account.AccountNumber,
		"routing_number": account.RoutingNumber,
		"account_type":   account.AccountType,
		"description":    fmt.Sprintf("Payout %s", payout.ID),
		"external_id":    payout.ID,
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", p.config.SponsorBankAPIURL+"/ach/transfers", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+p.config.SponsorBankAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("sponsor bank API error: %s", string(body))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.ID, nil
}

func validateACHRoutingNumber(routing string) error {
	if len(routing) != 9 {
		return fmt.Errorf("routing number must be 9 digits")
	}
	var sum int
	for i, ch := range routing[:8] {
		digit := int(ch - '0')
		switch i % 3 {
		case 0:
			sum += digit * 3
		case 1:
			sum += digit * 7
		case 2:
			sum += digit
		}
	}
	checkDigit := int(routing[8] - '0')
	if (sum+checkDigit)%10 != 0 {
		return fmt.Errorf("invalid routing number check digit")
	}
	return nil
}

// =============================================================================
// SEPA Credit Transfer Processor (ISO 20022 pain.001)
// =============================================================================

// SEPAConfig holds configuration for SEPA processing
type SEPAConfig struct {
	InitiatingPartyName string
	InitiatingPartyID   string
	BIC                 string
	IBAN                string
	CreditorAgentBIC    string
	Currency            string
	APIEndpoint         string
	APIKey              string
	UseFileUpload       bool
}

// SEPAPayoutProcessor handles SEPA Credit Transfer payouts
type SEPAPayoutProcessor struct {
	config SEPAConfig
	client *http.Client
	store  BankAccountStore
	db     *sql.DB
}

// NewSEPAPayoutProcessor creates a new SEPA processor
func NewSEPAPayoutProcessor(config SEPAConfig, store BankAccountStore) *SEPAPayoutProcessor {
	return &SEPAPayoutProcessor{
		config: config,
		store:  store,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
	}
}

// WithDB adds database connection for status tracking
func (p *SEPAPayoutProcessor) WithDB(db *sql.DB) *SEPAPayoutProcessor {
	p.db = db
	return p
}

// InitiatePayout initiates a SEPA Credit Transfer
func (p *SEPAPayoutProcessor) InitiatePayout(ctx context.Context, payout *Payout) (string, error) {
	account, err := p.store.GetBankAccount(ctx, payout.DestinationID)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve bank account: %w", err)
	}

	if account.Currency != "eur" {
		return "", fmt.Errorf("SEPA only supports EUR payouts, got %s", account.Currency)
	}

	if err := validateIBAN(account.IBAN); err != nil {
		return "", fmt.Errorf("invalid IBAN: %w", err)
	}

	if p.config.UseFileUpload {
		fileRef, err := p.generateSEPAFile(ctx, payout, account)
		if err != nil {
			return "", err
		}
		return fileRef, nil
	}

	return p.initiateSEPAviaAPI(ctx, payout, account)
}

// CancelPayout cancels a SEPA payout if still pending
func (p *SEPAPayoutProcessor) CancelPayout(ctx context.Context, payoutID string) error {
	if !p.config.UseFileUpload && p.config.APIEndpoint != "" {
		req, err := http.NewRequestWithContext(ctx, "POST",
			p.config.APIEndpoint+"/payments/"+payoutID+"/cancel", nil)
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)

		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to cancel SEPA transfer: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("bank cancel error: %s", string(body))
		}
		return nil
	}

	if p.db != nil {
		var fileStatus string
		err := p.db.QueryRowContext(ctx,
			"SELECT status FROM sepa_file_tracking WHERE payout_id = $1", payoutID).Scan(&fileStatus)
		if err == nil && fileStatus == "pending" {
			_, err = p.db.ExecContext(ctx,
				"UPDATE sepa_file_tracking SET status = 'canceled' WHERE payout_id = $1", payoutID)
			return err
		}
	}

	return fmt.Errorf("SEPA transfer already submitted to bank, cannot cancel")
}

// GetPayoutStatus returns the current status from bank
func (p *SEPAPayoutProcessor) GetPayoutStatus(ctx context.Context, payoutID string) (PayoutStatus, error) {
	if p.config.UseFileUpload && p.db != nil {
		var fileStatus string
		err := p.db.QueryRowContext(ctx,
			"SELECT status FROM sepa_file_tracking WHERE payout_id = $1", payoutID).Scan(&fileStatus)
		if err == nil {
			switch fileStatus {
			case "pending":
				return PayoutStatusPending, nil
			case "submitted":
				return PayoutStatusInTransit, nil
			case "settled", "executed":
				return PayoutStatusPaid, nil
			case "returned", "rejected", "failed":
				return PayoutStatusFailed, nil
			case "canceled":
				return PayoutStatusCanceled, nil
			}
		}
	}

	if !p.config.UseFileUpload && p.config.APIEndpoint != "" {
		req, err := http.NewRequestWithContext(ctx, "GET",
			p.config.APIEndpoint+"/payments/"+payoutID, nil)
		if err != nil {
			return PayoutStatusFailed, err
		}

		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)

		resp, err := p.client.Do(req)
		if err != nil {
			return PayoutStatusFailed, err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return PayoutStatusFailed, fmt.Errorf("failed to get payout status")
		}

		var result struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return PayoutStatusFailed, err
		}

		switch result.Status {
		case "executed", "settled", "completed":
			return PayoutStatusPaid, nil
		case "pending", "processing":
			return PayoutStatusInTransit, nil
		case "returned", "rejected", "failed":
			return PayoutStatusFailed, nil
		case "canceled":
			return PayoutStatusCanceled, nil
		default:
			return PayoutStatusInTransit, nil
		}
	}

	return PayoutStatusInTransit, nil
}

type SCTDocument struct {
	XMLName  xml.Name `xml:"Document"`
	Xmlns    string   `xml:"xmlns,attr"`
	MsgID    string   `xml:"CstmrCdtTrfInitn>GrpHdr>MsgId"`
	CreDtTm  string   `xml:"CstmrCdtTrfInitn>GrpHdr>CreDtTm"`
	NbOfTxs  string   `xml:"CstmrCdtTrfInitn>GrpHdr>NbOfTxs"`
	CtrlSum  string   `xml:"CstmrCdtTrfInitn>GrpHdr>CtrlSum"`
	InitgPty InitgPty `xml:"CstmrCdtTrfInitn>GrpHdr>InitgPty"`
	PmtInf   PmtInf   `xml:"CstmrCdtTrfInitn>PmtInf"`
}

type InitgPty struct {
	Nm string `xml:"Nm"`
	Id Id     `xml:"Id,omitempty"`
}

type Id struct {
	OrgId OrgId `xml:"OrgId,omitempty"`
}

type OrgId struct {
	Othr Othr `xml:"Othr"`
}

type Othr struct {
	Id string `xml:"Id"`
}

type PmtInf struct {
	PmtInfId    string        `xml:"PmtInfId"`
	PmtMtd      string        `xml:"PmtMtd"`
	BtchBookg   bool          `xml:"BtchBookg"`
	NbOfTxs     string        `xml:"NbOfTxs"`
	CtrlSum     string        `xml:"CtrlSum"`
	PmtTpInf    PmtTpInf      `xml:"PmtTpInf"`
	ReqdExctnDt string        `xml:"ReqdExctnDt"`
	Dbtr        Dbtr          `xml:"Dbtr"`
	DbtrAcct    DbtrAcct      `xml:"DbtrAcct"`
	DbtrAgt     DbtrAgt       `xml:"DbtrAgt"`
	ChrgBr      string        `xml:"ChrgBr"`
	CdtTrfTxInf []CdtTrfTxInf `xml:"CdtTrfTxInf"`
}

type PmtTpInf struct {
	SvcLvl SvcLvl `xml:"SvcLvl"`
}

type SvcLvl struct {
	Cd string `xml:"Cd"`
}

type Dbtr struct {
	Nm string `xml:"Nm"`
}

type DbtrAcct struct {
	Id AcctId `xml:"Id"`
}

type AcctId struct {
	IBAN string `xml:"IBAN"`
}

type DbtrAgt struct {
	FinInstnId FinInstnId `xml:"FinInstnId"`
}

type FinInstnId struct {
	BICFI string `xml:"BICFI,omitempty"`
}

type CdtTrfTxInf struct {
	PmtId    PmtId    `xml:"PmtId"`
	Amt      Amt      `xml:"Amt"`
	CdtrAgt  CdtrAgt  `xml:"CdtrAgt"`
	Cdtr     Cdtr     `xml:"Cdtr"`
	CdtrAcct CdtrAcct `xml:"CdtrAcct"`
	RmtInf   RmtInf   `xml:"RmtInf"`
}

type PmtId struct {
	InstrId    string `xml:"InstrId"`
	EndToEndId string `xml:"EndToEndId"`
}

type Amt struct {
	InstdAmt InstdAmt `xml:"InstdAmt"`
}

type InstdAmt struct {
	Value string `xml:",chardata"`
	Ccy   string `xml:"Ccy,attr"`
}

type CdtrAgt struct {
	FinInstnId FinInstnId `xml:"FinInstnId"`
}

type Cdtr struct {
	Nm string `xml:"Nm"`
}

type CdtrAcct struct {
	Id AcctId `xml:"Id"`
}

type RmtInf struct {
	Ustrd string `xml:"Ustrd"`
}

func (p *SEPAPayoutProcessor) generateSEPAFile(ctx context.Context, payout *Payout, account *BankAccount) (string, error) {
	now := time.Now().UTC()
	msgID := fmt.Sprintf("MSG%s%d", payout.ID, now.Unix())
	endToEndID := fmt.Sprintf("E2E%s", payout.ID)

	doc := SCTDocument{
		Xmlns:   "urn:iso:std:iso:20022:tech:xsd:pain.001.001.09",
		MsgID:   msgID,
		CreDtTm: now.Format("2006-01-02T15:04:05"),
		NbOfTxs: "1",
		CtrlSum: payout.Amount.String(),
		InitgPty: InitgPty{
			Nm: p.config.InitiatingPartyName,
		},
		PmtInf: PmtInf{
			PmtInfId:  msgID + "-P1",
			PmtMtd:    "TRF",
			BtchBookg: false,
			NbOfTxs:   "1",
			CtrlSum:   payout.Amount.String(),
			PmtTpInf: PmtTpInf{
				SvcLvl: SvcLvl{Cd: "SEPA"},
			},
			ReqdExctnDt: now.Add(24 * time.Hour).Format("2006-01-02"),
			Dbtr:        Dbtr{Nm: p.config.InitiatingPartyName},
			DbtrAcct:    DbtrAcct{Id: AcctId{IBAN: p.config.IBAN}},
			DbtrAgt:     DbtrAgt{FinInstnId: FinInstnId{BICFI: p.config.BIC}},
			ChrgBr:      "SLEV",
			CdtTrfTxInf: []CdtTrfTxInf{
				{
					PmtId: PmtId{
						InstrId:    payout.ID,
						EndToEndId: endToEndID,
					},
					Amt: Amt{
						InstdAmt: InstdAmt{
							Value: payout.Amount.String(),
							Ccy:   "EUR",
						},
					},
					CdtrAgt: CdtrAgt{
						FinInstnId: FinInstnId{BICFI: account.BIC},
					},
					Cdtr:     Cdtr{Nm: account.BankName},
					CdtrAcct: CdtrAcct{Id: AcctId{IBAN: account.IBAN}},
					RmtInf:   RmtInf{Ustrd: fmt.Sprintf("Payout %s", payout.ID)},
				},
			},
		},
	}

	output, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal SEPA XML: %w", err)
	}

	output = append([]byte(xml.Header), output...)
	fileRef := fmt.Sprintf("SEPA%s%d", payout.ID, now.Unix())
	return fileRef, nil
}

func (p *SEPAPayoutProcessor) initiateSEPAviaAPI(ctx context.Context, payout *Payout, account *BankAccount) (string, error) {
	reqBody := map[string]interface{}{
		"amount":         payout.Amount.String(),
		"currency":       payout.Currency,
		"iban":           account.IBAN,
		"bic":            account.BIC,
		"account_holder": account.BankName,
		"description":    fmt.Sprintf("Payout %s", payout.ID),
		"end_to_end_id":  fmt.Sprintf("E2E%s", payout.ID),
		"requested_date": time.Now().Add(24 * time.Hour).Format("2006-01-02"),
		"scheme":         "SEPA",
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", p.config.APIEndpoint+"/payments/sepa", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("SEPA API error: %s", string(body))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.ID, nil
}

func validateIBAN(iban string) error {
	iban = strings.ReplaceAll(iban, " ", "")
	if len(iban) < 15 || len(iban) > 34 {
		return fmt.Errorf("IBAN length must be between 15 and 34 characters")
	}
	re := regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{1,30}$`)
	if !re.MatchString(iban) {
		return fmt.Errorf("invalid IBAN format")
	}
	return nil
}

// =============================================================================
// Faster Payments Processor (UK)
// =============================================================================

// FasterPaymentsConfig holds configuration for UK Faster Payments
type FasterPaymentsConfig struct {
	APIEndpoint       string
	APIKey            string
	OriginatorName    string
	OriginatorAccount string
	SortCode          string
	AccountNumber     string
	Sandbox           bool
}

// FasterPaymentsPayoutProcessor handles UK Faster Payments
type FasterPaymentsPayoutProcessor struct {
	config FasterPaymentsConfig
	client *http.Client
	store  BankAccountStore
	db     *sql.DB
}

// NewFasterPaymentsPayoutProcessor creates a new Faster Payments processor
func NewFasterPaymentsPayoutProcessor(config FasterPaymentsConfig, store BankAccountStore) *FasterPaymentsPayoutProcessor {
	return &FasterPaymentsPayoutProcessor{
		config: config,
		store:  store,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
	}
}

// WithDB adds database connection for status tracking
func (p *FasterPaymentsPayoutProcessor) WithDB(db *sql.DB) *FasterPaymentsPayoutProcessor {
	p.db = db
	return p
}

// InitiatePayout initiates a Faster Payments payout
func (p *FasterPaymentsPayoutProcessor) InitiatePayout(ctx context.Context, payout *Payout) (string, error) {
	account, err := p.store.GetBankAccount(ctx, payout.DestinationID)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve bank account: %w", err)
	}

	if account.Currency != "gbp" {
		return "", fmt.Errorf("Faster Payments only supports GBP payouts, got %s", account.Currency)
	}

	if err := validateUKSortCode(account.SortCode); err != nil {
		return "", fmt.Errorf("invalid sort code: %w", err)
	}

	if err := validateUKAccountNumber(account.AccountNumber); err != nil {
		return "", fmt.Errorf("invalid account number: %w", err)
	}

	reqBody := map[string]interface{}{
		"amount":                      payout.Amount.String(),
		"currency":                    payout.Currency,
		"sort_code":                   account.SortCode,
		"account_number":              account.AccountNumber,
		"account_name":                account.BankName,
		"reference":                   fmt.Sprintf("PAYOUT-%s", payout.ID),
		"originator_name":             p.config.OriginatorName,
		"originator_sort_code":        p.config.SortCode,
		"originator_account_number": p.config.AccountNumber,
		"scheme":                      "FPS",
	}

	jsonBody, _ := json.Marshal(reqBody)
	url := p.config.APIEndpoint + "/payments/faster-payments"
	if p.config.Sandbox {
		url = p.config.APIEndpoint + "/sandbox/payments/faster-payments"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency-Key", payout.ID)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Faster Payments API error: %s", string(body))
	}

	var result struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		SubmittedAt string `json:"submitted_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.ID, nil
}

// CancelPayout cancels a Faster Payments payout if still pending
func (p *FasterPaymentsPayoutProcessor) CancelPayout(ctx context.Context, payoutID string) error {
	return fmt.Errorf("Faster Payments cannot be cancelled after submission")
}

// GetPayoutStatus returns the current status from FPSP
func (p *FasterPaymentsPayoutProcessor) GetPayoutStatus(ctx context.Context, payoutID string) (PayoutStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.config.APIEndpoint+"/payments/"+payoutID, nil)
	if err != nil {
		return PayoutStatusFailed, err
	}

	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return PayoutStatusFailed, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return PayoutStatusFailed, fmt.Errorf("failed to get payout status")
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return PayoutStatusFailed, err
	}

	switch result.Status {
	case "completed", "settled":
		return PayoutStatusPaid, nil
	case "rejected", "failed":
		return PayoutStatusFailed, nil
	default:
		return PayoutStatusInTransit, nil
	}
}

func validateUKSortCode(sortCode string) error {
	sortCode = strings.ReplaceAll(sortCode, "-", "")
	if len(sortCode) != 6 {
		return fmt.Errorf("sort code must be 6 digits")
	}
	if _, err := strconv.Atoi(sortCode); err != nil {
		return fmt.Errorf("sort code must contain only digits")
	}
	return nil
}

func validateUKAccountNumber(accountNumber string) error {
	if len(accountNumber) < 6 || len(accountNumber) > 10 {
		return fmt.Errorf("account number must be between 6 and 10 digits")
	}
	if _, err := strconv.Atoi(accountNumber); err != nil {
		return fmt.Errorf("account number must contain only digits")
	}
	return nil
}

// =============================================================================
// Utility Functions
// =============================================================================

func padRight(s string, length int) string {
	if len(s) > length {
		return s[:length]
	}
	return s + strings.Repeat(" ", length-len(s))
}

// Verify interface implementations
var _ PayoutProcessor = (*ACHPayoutProcessor)(nil)
var _ PayoutProcessor = (*SEPAPayoutProcessor)(nil)
var _ PayoutProcessor = (*FasterPaymentsPayoutProcessor)(nil)
