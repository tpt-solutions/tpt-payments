// tpt-payments/backend/internal/disputes/reason_codes.go
package disputes

import "fmt"

// VisaReasonCode represents a Visa chargeback reason code
type VisaReasonCode struct {
	Code        string
	Reason      string
	Description string
	Examples    []string
}

// MastercardReasonCode represents a Mastercard chargeback reason code
type MastercardReasonCode struct {
	Code        string
	Reason      string
	Description string
	Category    string // Fraud, Processing Error, etc.
}

// VisaReasonCodes list (simplified, common codes)
var VisaReasonCodes = map[string]VisaReasonCode{
	"10.1": {
		Code:        "10.1",
		Reason:      "Fraud — Card Absent Environment",
		Description: "Transaction occurred in a card-absent environment (e.g., internet, mail/phone order) and the cardholder claims they did not authorize the transaction.",
		Examples:    []string{"Card-not-present transaction", "Internet transaction"},
	},
	"10.2": {
		Code:        "10.2",
		Reason:      "Fraud — Card Present Environment",
		Description: "Transaction occurred in a card-present environment and the cardholder claims they did not authorize the transaction.",
		Examples:    []string{"Counterfeit card", "Lost or stolen card"},
	},
	"10.3": {
		Code:        "10.3",
		Reason:      "Fraud — Card Absent Environment (Recurring)",
		Description: "Recurring transaction that the cardholder claims they did not authorize.",
		Examples:    []string{"Recurring payment", "Subscription"},
	},
	"10.4": {
		Code:        "10.4",
		Reason:      "Fraud — Card Absent Environment (Installment)",
		Description: "Installment transaction that the cardholder claims they did not authorize.",
		Examples:    []string{"Installment plan"},
	},
	"10.5": {
		Code:        "10.5",
		Reason:      "Fraud — Card Present Environment (Counterfeit)",
		Description: "Transaction involving a counterfeit card.",
		Examples:    []string{"Counterfeit card detected"},
	},
	"11.1": {
		Code:        "11.1",
		Reason:      "Cancelled Recurring Transaction",
		Description: "Cardholder claims a recurring transaction was cancelled but still processed.",
		Examples:    []string{"Cancelled subscription still billed"},
	},
	"11.2": {
		Code:        "11.2",
		Reason:      "Cancelled Installment Transaction",
		Description: "Cardholder claims an installment transaction was cancelled but still processed.",
		Examples:    []string{"Cancelled installment plan"},
	},
	"11.3": {
		Code:        "11.3",
		Reason:      "Cancelled Card-Absent Transaction",
		Description: "Cardholder claims a card-absent transaction was cancelled but still processed.",
		Examples:    []string{"Cancelled internet order"},
	},
	"12.1": {
		Code:        "12.1",
		Reason:      "Late Presentment",
		Description: "Transaction was not presented to Visa within the required time frame.",
		Examples:    []string{"Transaction submitted after deadline"},
	},
	"12.2": {
		Code:        "12.2",
		Reason:      "Incorrect Transaction Amount",
		Description: "Transaction amount differs from what was authorized or agreed.",
		Examples:    []string{"Overcharged", "Incorrect currency conversion"},
	},
	"12.3": {
		Code:        "12.3",
		Reason:      "Incorrect Currency",
		Description: "Transaction was processed in wrong currency.",
		Examples:    []string{"Currency mismatch"},
	},
	"12.4": {
		Code:        "12.4",
		Reason:      "Duplicate Processing",
		Description: "Same transaction was processed more than once.",
		Examples:    []string{"Double charged"},
	},
	"12.5": {
		Code:        "12.5",
		Reason:      "Missing Auth",
		Description: "Transaction lacks proper authorization.",
		Examples:    []string{"No authorization obtained"},
	},
	"12.6": {
		Code:        "12.6",
		Reason:      "Late Delivery",
		Description: "Goods or services were not delivered within the promised timeframe.",
		Examples:    []string{"Shipping delay", "Service not provided on time"},
	},
	"12.7": {
		Code:        "12.7",
		Reason:      "Incorrect Billing",
		Description: "Cardholder was billed incorrectly (e.g., wrong item, wrong quantity).",
		Examples:    []string{"Wrong item shipped", "Incorrect quantity"},
	},
	"13.1": {
		Code:        "13.1",
		Reason:      "Merchandise/Services Not Received",
		Description: "Cardholder claims they did not receive the merchandise or services.",
		Examples:    []string{"Item not delivered", "Service not rendered"},
	},
	"13.2": {
		Code:        "13.2",
		Reason:      "Cancelled Service",
		Description: "Cardholder claims they cancelled a service but still were billed.",
		Examples:    []string{"Cancelled subscription", "Cancelled booking"},
	},
	"13.3": {
		Code:        "13.3",
		Reason:      "Not as Described",
		Description: "Merchandise or services received are not as described or defective.",
		Examples:    []string{"Item not as advertised", "Defective product"},
	},
	"13.4": {
		Code:        "13.4",
		Reason:      "Counterfeit Merchandise",
		Description: "Merchandise received is counterfeit.",
		Examples:    []string{"Fake goods"},
	},
	"13.5": {
		Code:        "13.5",
		Reason:      "Misrepresentation",
		Description: "Merchant misrepresented the product or service.",
		Examples:    []string{"False advertising", "Bait and switch"},
	},
	"13.6": {
		Code:        "13.6",
		Reason:      "Credit Not Processed",
		Description: "Cardholder returned merchandise but credit was not processed.",
		Examples:    []string{"Return not refunded"},
	},
	"13.7": {
		Code:        "13.7",
		Reason:      "Cancelled Recurring Transaction (Visa)",
		Description: "Recurring transaction cancelled but still processed (similar to 11.1).",
		Examples:    []string{"Cancelled subscription"},
	},
	"13.8": {
		Code:        "13.8",
		Reason:      "Non-Receipt of Cash or Merchandise with ATM or Dynamic Currency Conversion",
		Description: "Cash not dispensed from ATM or merchandise not received in dynamic currency conversion.",
		Examples:    []string{"ATM didn't dispense cash"},
	},
	"14.1": {
		Code:        "14.1",
		Reason:      "Cardholder Dispute — Credit Not Issued",
		Description: "Cardholder claims a credit was not issued for a returned item.",
		Examples:    []string{"Refund not received"},
	},
	"14.2": {
		Code:        "14.2",
		Reason:      "Cardholder Dispute — Credit Posted to Wrong Account",
		Description: "Credit was issued to wrong account.",
		Examples:    []string{"Refund to wrong card"},
	},
	"14.3": {
		Code:        "14.3",
		Reason:      "Cardholder Dispute — Not as Described",
		Description: "Goods or services not as described (similar to 13.3).",
		Examples:    []string{"Item misrepresented"},
	},
	"14.4": {
		Code:        "14.4",
		Reason:      "Cardholder Dispute — Duplicate Processing",
		Description: "Transaction was processed twice (similar to 12.4).",
		Examples:    []string{"Double billed"},
	},
	"14.5": {
		Code:        "14.5",
		Reason:      "Cardholder Dispute — Paid Through Other Means",
		Description: "Cardholder claims they paid through other means (e.g., cash, check).",
		Examples:    []string{"Already paid via bank transfer"},
	},
	"15.1": {
		Code:        "15.1",
		Reason:      "Visa Fraud Monitoring — Card Absent",
		Description: "Visa fraud monitoring program: card-absent fraud.",
		Examples:    []string{"High fraud rate"},
	},
	"15.2": {
		Code:        "15.2",
		Reason:      "Visa Fraud Monitoring — Card Present",
		Description: "Visa fraud monitoring program: card-present fraud.",
		Examples:    []string{"Counterfeit fraud"},
	},
}

// MastercardReasonCodes list (simplified, common codes)
var MastercardReasonCodes = map[string]MastercardReasonCode{
	"4837": {
		Code:        "4837",
		Reason:      "Fraudulent Transaction - Card Absent Environment",
		Description: "Transaction occurred in a card-absent environment and the cardholder claims they did not authorize it.",
		Category:    "Fraud",
	},
	"4863": {
		Code:        "4863",
		Reason:      "Cardholder Does Not Recognize Transaction",
		Description: "Cardholder does not recognize the transaction on their statement.",
		Category:    "Fraud",
	},
	"4807": {
		Code:        "4807",
		Reason:      "Warning Bulletin File (Card Reported Lost or Stolen)",
		Description: "Transaction was processed after the card was reported lost or stolen.",
		Category:    "Fraud",
	},
	"4812": {
		Code:        "4812",
		Reason:      "Account Takeover",
		Description: "Fraudulent transaction resulting from unauthorized access to cardholder's account.",
		Category:    "Fraud",
	},
	"4823": {
		Code:        "4823",
		Reason:      "Fraudulent Transaction - Card Present Environment",
		Description: "Card-present transaction that was fraudulent.",
		Category:    "Fraud",
	},
	"4840": {
		Code:        "4840",
		Reason:      "Fraudulent Transaction - Counterfeit Card",
		Description: "Transaction involved a counterfeit card.",
		Category:    "Fraud",
	},
	"4841": {
		Code:        "4841",
		Reason:      "Fraudulent Transaction - Card Not Received",
		Description: "Cardholder claims they never received the card (new or reissued).",
		Category:    "Fraud",
	},
	"4842": {
		Code:        "4842",
		Reason:      "Fraudulent Transaction - Card Issued in Response to Fraudulent Application",
		Description: "Card was issued based on a fraudulent application.",
		Category:    "Fraud",
	},
	"4853": {
		Code:        "4853",
		Reason:      "Cardholder Dispute - Defective/Not as Described",
		Description: "Goods or services are defective or not as described.",
		Category:    "Processing Error",
	},
	"4854": {
		Code:        "4854",
		Reason:      "Cardholder Dispute - Not Received",
		Description: "Cardholder claims they did not receive the merchandise or services.",
		Category:    "Processing Error",
	},
	"4855": {
		Code:        "4855",
		Reason:      "Non-Receipt of Cash or Merchandise with ATM",
		Description: "Cash not dispensed from ATM or merchandise not received.",
		Category:    "Processing Error",
	},
	"4856": {
		Code:        "4856",
		Reason:      "Cardholder Dispute - Credit Not Processed",
		Description: "Returned merchandise but credit not processed.",
		Category:    "Processing Error",
	},
	"4857": {
		Code:        "4857",
		Reason:      "Cardholder Dispute - Cancelled Recurring Transaction",
		Description: "Recurring transaction cancelled but still processed.",
		Category:    "Processing Error",
	},
	"4859": {
		Code:        "4859",
		Reason:      "Addendum - Fraudulent Transaction",
		Description: "Additional information supporting a fraud dispute.",
		Category:    "Fraud",
	},
	"4860": {
		Code:        "4860",
		Reason:      "Cardholder Dispute - Goods or Services Not Provided",
		Description: "Goods or services not provided as agreed.",
		Category:    "Processing Error",
	},
	"4861": {
		Code:        "4861",
		Reason:      "Canceled Cardholder Dispute - Goods or Services Not Provided",
		Description: "Cardholder cancelled but still billed for goods/services.",
		Category:    "Processing Error",
	},
	"4862": {
		Code:        "4862",
		Reason:      "Cardholder Dispute - Counterfeit Transaction",
		Description: "Transaction involved counterfeit card.",
		Category:    "Fraud",
	},
	"4865": {
		Code:        "4865",
		Reason:      "Cardholder Dispute - Duplicate Processing",
		Description: "Transaction processed more than once.",
		Category:    "Processing Error",
	},
	"4866": {
		Code:        "4866",
		Reason:      "Cardholder Dispute - Paid Through Other Means",
		Description: "Cardholder claims they paid through other means.",
		Category:    "Processing Error",
	},
	"4870": {
		Code:        "4870",
		Reason:      "Chip Liability Shift",
		Description: "Liability shift due to chip transaction not processed correctly.",
		Category:    "Processing Error",
	},
	"4871": {
		Code:        "4871",
		Reason:      "Chip/PIN Liability Shift",
		Description: "Liability shift due to chip/PIN transaction not processed correctly.",
		Category:    "Processing Error",
	},
}

// GetVisaReasonCode returns details for a Visa reason code
func GetVisaReasonCode(code string) (VisaReasonCode, error) {
	rc, ok := VisaReasonCodes[code]
	if !ok {
		return VisaReasonCode{}, fmt.Errorf("unknown Visa reason code: %s", code)
	}
	return rc, nil
}

// GetMastercardReasonCode returns details for a Mastercard reason code
func GetMastercardReasonCode(code string) (MastercardReasonCode, error) {
	rc, ok := MastercardReasonCodes[code]
	if !ok {
		return MastercardReasonCode{}, fmt.Errorf("unknown Mastercard reason code: %s", code)
	}
	return rc, nil
}

// GetAllVisaReasonCodes returns all Visa reason codes
func GetAllVisaReasonCodes() map[string]VisaReasonCode {
	return VisaReasonCodes
}

// GetAllMastercardReasonCodes returns all Mastercard reason codes
func GetAllMastercardReasonCodes() map[string]MastercardReasonCode {
	return MastercardReasonCodes
}

// SuggestEvidenceForVisa suggests evidence for a Visa dispute reason code
func SuggestEvidenceForVisa(reasonCode string) []string {
	switch reasonCode {
	case "10.1", "10.2", "10.3", "10.4", "10.5":
		return []string{"customer_signature", "access_activity_log", "shipping_document"}
	case "11.1", "11.2", "11.3":
		return []string{"cancellation_confirmation", "customer_communication"}
	case "12.1", "12.2", "12.3", "12.4", "12.5":
		return []string{"receipt", "transaction_log", "customer_communication"}
	case "13.1", "13.2", "13.3", "13.4", "13.5", "13.6", "13.7", "13.8":
		return []string{"shipping_document", "product_description", "customer_communication", "refund_policy"}
	case "14.1", "14.2", "14.3", "14.4", "14.5":
		return []string{"refund_receipt", "customer_communication", "refund_policy"}
	default:
		return []string{"uncategorized_text", "customer_communication"}
	}
}

// SuggestEvidenceForMastercard suggests evidence for a Mastercard dispute reason code
func SuggestEvidenceForMastercard(reasonCode string) []string {
	switch reasonCode {
	case "4837", "4863", "4807", "4812", "4823", "4840", "4841", "4842", "4859", "4862":
		return []string{"customer_signature", "access_activity_log", "shipping_document"}
	case "4853", "4854", "4855", "4856", "4860", "4861", "4865", "4866":
		return []string{"shipping_document", "product_description", "customer_communication", "refund_policy"}
	case "4870", "4871":
		return []string{"terminal_log", "chip_transaction_evidence"}
	default:
		return []string{"uncategorized_text", "customer_communication"}
	}
}