// tpt-payments/backend/internal/acquiring/iso8583/parser.go
package iso8583

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Message represents an ISO 8583 financial transaction message
type Message struct {
	MTI        MTI
	Fields     map[int][]byte
	Raw        []byte
}

// MTI (Message Type Indicator) represents the type of ISO 8583 message
type MTI struct {
	Version    byte // 0xxx, 1xxx, 2xxx, etc.
	Class      byte // 0xxx = reserved, 1xxx = auth, 2xxx = financial, 3xxx = file action, 4xxx = reversal, etc.
	Function   byte // 0 = request, 1 = response
	Origin     byte // 0 = acquirer, 1 = issuer
}

// ParseMTI parses a 4-byte MTI
func ParseMTI(data []byte) (MTI, error) {
	if len(data) != 4 {
		return MTI{}, fmt.Errorf("MTI must be 4 bytes, got %d", len(data))
	}

	return MTI{
		Version:  data[0] - '0',
		Class:    data[1] - '0',
		Function: data[2] - '0',
		Origin:   data[3] - '0',
	}, nil
}

// String returns the MTI as a 4-digit string
func (m MTI) String() string {
	return fmt.Sprintf("%d%d%d%d", m.Version, m.Class, m.Function, m.Origin)
}

// IsRequest returns true if this is a request message
func (m MTI) IsRequest() bool {
	return m.Function == 0
}

// IsResponse returns true if this is a response message
func (m MTI) IsResponse() bool {
	return m.Function == 1
}

// Common MTI values
const (
	MTIAuthorizationRequest   = "0100" // Auth request
	MTIAuthorizationResponse  = "0110" // Auth response
	MTICaptureRequest        = "0220" // Financial transaction (capture)
	MTICaptureResponse       = "0230" // Financial transaction response
	MTIReversalRequest      = "0400" // Reversal request
	MTIReversalResponse     = "0410" // Reversal response
	MTIRefundRequest         = "0200" // Refund request (some implementations)
	MTISettlementRequest    = "0520" // Settlement request
	MTISettlementResponse   = "0530" // Settlement response
)

// Parser handles ISO 8583 message parsing
type Parser struct {
	Bitmap     []byte
	Fields     map[int]FieldDescriptor
}

// FieldDescriptor describes an ISO 8583 field
type FieldDescriptor struct {
	ID         int
	Type       FieldType
	Length     int
	MaxLength  int
	Variable   bool
}

// FieldType represents the data type of an ISO 8583 field
type FieldType int

const (
	FieldTypeNumeric     FieldType = iota // N - numeric
	FieldTypeAlphaNumeric         // AN - alphanumeric
	FieldTypeAlphaNumericSpecial // ANS - alphanumeric + special
	FieldTypeBinary                // B - binary
	FieldTypeLLVAR                // LLVAR - variable length (2-digit length)
	FieldTypeLLLVAR               // LLLVAR - variable length (3-digit length)
)

// DefaultFieldDescriptors returns the standard ISO 8583 field descriptors
func DefaultFieldDescriptors() map[int]FieldDescriptor {
	return map[int]FieldDescriptor{
		1:  {ID: 1, Type: FieldTypeBinary, Length: 8, MaxLength: 8},       // Bitmap (primary)
		2:  {ID: 2, Type: FieldTypeNumeric, Length: 0, MaxLength: 19, Variable: true}, // Primary account number (PAN)
		3:  {ID: 3, Type: FieldTypeNumeric, Length: 6, MaxLength: 6},       // Processing code
		4:  {ID: 4, Type: FieldTypeNumeric, Length: 12, MaxLength: 12},      // Amount, transaction
		5:  {ID: 5, Type: FieldTypeNumeric, Length: 12, MaxLength: 12},      // Amount, settlement
		6:  {ID: 6, Type: FieldTypeNumeric, Length: 12, MaxLength: 12},      // Amount, cardholder billing
		7:  {ID: 7, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},      // Transmission date & time
		8:  {ID: 8, Type: FieldTypeNumeric, Length: 8, MaxLength: 8},       // Cardholder billing fee amount
		9:  {ID: 9, Type: FieldTypeNumeric, Length: 8, MaxLength: 8},       // Conversion rate, settlement
		10: {ID: 10, Type: FieldTypeNumeric, Length: 8, MaxLength: 8},       // Conversion rate, cardholder billing
		11: {ID: 11, Type: FieldTypeNumeric, Length: 6, MaxLength: 6},       // Systems trace audit number (STAN)
		12: {ID: 12, Type: FieldTypeNumeric, Length: 6, MaxLength: 6},       // Local transaction time
		13: {ID: 13, Type: FieldTypeNumeric, Length: 4, MaxLength: 4},       // Local transaction date
		14: {ID: 14, Type: FieldTypeNumeric, Length: 4, MaxLength: 4},       // Expiration date
		15: {ID: 15, Type: FieldTypeNumeric, Length: 4, MaxLength: 4},       // Settlement date
		16: {ID: 16, Type: FieldTypeNumeric, Length: 4, MaxLength: 4},       // Conversion date
		17: {ID: 17, Type: FieldTypeNumeric, Length: 4, MaxLength: 4},       // Capture date
		18: {ID: 18, Type: FieldTypeNumeric, Length: 4, MaxLength: 4},       // Merchant type
		19: {ID: 19, Type: FieldTypeNumeric, Length: 3, MaxLength: 3},       // Acquiring institution country code
		20: {ID: 20, Type: FieldTypeNumeric, Length: 3, MaxLength: 3},       // PAN extended, country code
		21: {ID: 21, Type: FieldTypeNumeric, Length: 3, MaxLength: 3},       // Forwarding institution country code
		22: {ID: 22, Type: FieldTypeAlphaNumeric, Length: 3, MaxLength: 3},     // POS entry mode
		23: {ID: 23, Type: FieldTypeNumeric, Length: 3, MaxLength: 3},       // Card sequence number
		24: {ID: 24, Type: FieldTypeNumeric, Length: 3, MaxLength: 3},       // Function code (network)
		25: {ID: 25, Type: FieldTypeNumeric, Length: 2, MaxLength: 2},       // POS condition code
		26: {ID: 26, Type: FieldTypeNumeric, Length: 2, MaxLength: 2},       // POS PIN capture code
		27: {ID: 27, Type: FieldTypeNumeric, Length: 1, MaxLength: 1},       // Authorization ID response length
		28: {ID: 28, Type: FieldTypeNumeric, Length: 8, MaxLength: 8},       // Amount, transaction fee
		29: {ID: 29, Type: FieldTypeNumeric, Length: 8, MaxLength: 8},       // Amount, settlement fee
		30: {ID: 30, Type: FieldTypeNumeric, Length: 8, MaxLength: 8},       // Amount, transaction processing fee
		31: {ID: 31, Type: FieldTypeNumeric, Length: 8, MaxLength: 8},       // Amount, settlement processing fee
		32: {ID: 32, Type: FieldTypeNumeric, Length: 0, MaxLength: 11, Variable: true}, // Acquiring institution ID code
		33: {ID: 33, Type: FieldTypeNumeric, Length: 0, MaxLength: 11, Variable: true}, // Forwarding institution ID code
		34: {ID: 34, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 28, Variable: true}, // PAN extended
		35: {ID: 35, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 37, Variable: true}, // Track 2 data
		36: {ID: 36, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 104, Variable: true}, // Track 3 data
		37: {ID: 37, Type: FieldTypeAlphaNumeric, Length: 12, MaxLength: 12},    // Retrieval reference number
		38: {ID: 38, Type: FieldTypeAlphaNumeric, Length: 6, MaxLength: 6},     // Authorization ID response
		39: {ID: 39, Type: FieldTypeAlphaNumeric, Length: 2, MaxLength: 2},     // Response code
		40: {ID: 40, Type: FieldTypeAlphaNumeric, Length: 3, MaxLength: 3},     // Service restriction code
		41: {ID: 41, Type: FieldTypeAlphaNumeric, Length: 8, MaxLength: 8},     // Card acceptor terminal ID
		42: {ID: 42, Type: FieldTypeAlphaNumeric, Length: 15, MaxLength: 15},    // Card acceptor ID code
		43: {ID: 43, Type: FieldTypeAlphaNumeric, Length: 40, MaxLength: 40},    // Card acceptor name/location
		44: {ID: 44, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 25, Variable: true}, // Additional response data
		45: {ID: 45, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 76, Variable: true}, // Track 1 data
		46: {ID: 46, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Additional data (ISO)
		47: {ID: 47, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Additional data (national)
		48: {ID: 48, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Additional data (private)
		49: {ID: 49, Type: FieldTypeNumeric, Length: 3, MaxLength: 3},       // Currency code, transaction
		50: {ID: 50, Type: FieldTypeNumeric, Length: 3, MaxLength: 3},       // Currency code, settlement
		51: {ID: 51, Type: FieldTypeNumeric, Length: 3, MaxLength: 3},       // Currency code, cardholder billing
		52: {ID: 52, Type: FieldTypeBinary, Length: 8, MaxLength: 8},       // PIN data
		53: {ID: 53, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Security related control info
		54: {ID: 54, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 120, Variable: true}, // Additional amounts
		55: {ID: 55, Type: FieldTypeBinary, Length: 0, MaxLength: 999, Variable: true}, // IC card system related data
		56: {ID: 56, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Original data elements
		57: {ID: 57, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (national)
		58: {ID: 58, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (national)
		59: {ID: 59, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (national)
		60: {ID: 60, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (national)
		61: {ID: 61, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (private)
		62: {ID: 62, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (private)
		63: {ID: 63, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (private)
		64: {ID: 64, Type: FieldTypeBinary, Length: 8, MaxLength: 8},       // Message authentication code (MAC)
		65: {ID: 65, Type: FieldTypeBinary, Length: 8, MaxLength: 8},       // Bitmap (extended)
		66: {ID: 66, Type: FieldTypeNumeric, Length: 1, MaxLength: 1},       // Settlement code
		67: {ID: 67, Type: FieldTypeNumeric, Length: 2, MaxLength: 2},       // Extended payment code
		68: {ID: 68, Type: FieldTypeNumeric, Length: 3, MaxLength: 3},       // Receiving institution country code
		69: {ID: 69, Type: FieldTypeNumeric, Length: 3, MaxLength: 3},       // Settlement institution country code
		70: {ID: 70, Type: FieldTypeNumeric, Length: 3, MaxLength: 3},       // Network management info code
		71: {ID: 71, Type: FieldTypeNumeric, Length: 4, MaxLength: 4},       // Message number
		72: {ID: 72, Type: FieldTypeNumeric, Length: 0, MaxLength: 999, Variable: true}, // Data record (national)
		73: {ID: 73, Type: FieldTypeNumeric, Length: 6, MaxLength: 6},       // Date, action (action date)
		74: {ID: 74, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},     // Credits, number
		75: {ID: 75, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},     // Credits, reversal number
		76: {ID: 76, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},     // Debits, number
		77: {ID: 77, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},     // Debits, reversal number
		78: {ID: 78, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},     // Transfer, number
		79: {ID: 79, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},     // Transfer, reversal number
		80: {ID: 80, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},     // Credits, amount
		81: {ID: 81, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},     // Credits, reversal amount
		82: {ID: 82, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},     // Debits, amount
		83: {ID: 83, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},     // Debits, reversal amount
		84: {ID: 84, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},     // Transfer, amount
		85: {ID: 85, Type: FieldTypeNumeric, Length: 10, MaxLength: 10},     // Transfer, reversal amount
		86: {ID: 86, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Credits, count (settlement)
		87: {ID: 87, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Credits, reversal count (settlement)
		88: {ID: 88, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Debits, count (settlement)
		89: {ID: 89, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Debits, reversal count (settlement)
		90: {ID: 90, Type: FieldTypeNumeric, Length: 42, MaxLength: 42},     // Original data elements
		91: {ID: 91, Type: FieldTypeAlphaNumeric, Length: 1, MaxLength: 1},     // File update code
		92: {ID: 92, Type: FieldTypeNumeric, Length: 2, MaxLength: 2},       // File security code
		93: {ID: 93, Type: FieldTypeAlphaNumeric, Length: 5, MaxLength: 5},     // Response indicator
		94: {ID: 94, Type: FieldTypeAlphaNumeric, Length: 7, MaxLength: 7},     // Service indicator
		95: {ID: 95, Type: FieldTypeBinary, Length: 42, MaxLength: 42},     // Replacement amounts
		96: {ID: 96, Type: FieldTypeBinary, Length: 16, MaxLength: 16},     // Message security code
		97: {ID: 97, Type: FieldTypeNumeric, Length: 17, MaxLength: 17},     // Net settlement indicator
		98: {ID: 98, Type: FieldTypeAlphaNumeric, Length: 25, MaxLength: 25},    // Payee
		99: {ID: 99, Type: FieldTypeNumeric, Length: 11, MaxLength: 11},      // Settlement institution ID
		100: {ID: 100, Type: FieldTypeNumeric, Length: 0, MaxLength: 11, Variable: true}, // Receiving institution ID
		101: {ID: 101, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 17, Variable: true}, // File name
		102: {ID: 102, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 28, Variable: true}, // Account identification 1
		103: {ID: 103, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 28, Variable: true}, // Account identification 2
		104: {ID: 104, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 100, Variable: true}, // Transaction description
		105: {ID: 105, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Credits, amount (settlement)
		106: {ID: 106, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Credits, reversal amount (settlement)
		107: {ID: 107, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Debits, amount (settlement)
		108: {ID: 108, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Debits, reversal amount (settlement)
		109: {ID: 109, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Transfer, amount (settlement)
		110: {ID: 110, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Transfer, reversal amount (settlement)
		111: {ID: 111, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Credits, count (settlement, 2)
		112: {ID: 112, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Credits, reversal count (settlement, 2)
		113: {ID: 113, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Debits, count (settlement, 2)
		114: {ID: 114, Type: FieldTypeNumeric, Length: 16, MaxLength: 16},     // Debits, reversal count (settlement, 2)
		115: {ID: 115, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (national use)
		116: {ID: 116, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (national use)
		117: {ID: 117, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (national use)
		118: {ID: 118, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (national use)
		119: {ID: 119, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (national use)
		120: {ID: 120, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (private use)
		121: {ID: 121, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (private use)
		122: {ID: 122, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (private use)
		123: {ID: 123, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (private use)
		124: {ID: 124, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (private use)
		125: {ID: 125, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (private use)
		126: {ID: 126, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (private use)
		127: {ID: 127, Type: FieldTypeAlphaNumeric, Length: 0, MaxLength: 999, Variable: true}, // Reserved (private use)
		128: {ID: 128, Type: FieldTypeBinary, Length: 8, MaxLength: 8},       // Message authentication code
	}
}

// Parse parses an ISO 8583 message from raw bytes
func (p *Parser) Parse(data []byte) (*Message, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("message too short for MTI")
	}

	// Parse MTI (first 4 bytes)
	mti, err := ParseMTI(data[:4])
	if err != nil {
		return nil, err
	}

	msg := &Message{
		MTI:    mti,
		Fields: make(map[int][]byte),
		Raw:    data,
	}

	offset := 4

	// Parse primary bitmap (bytes 4-11)
	if len(data) < offset+8 {
		return nil, fmt.Errorf("message too short for primary bitmap")
	}
	primaryBitmap := data[offset : offset+8]
	offset += 8

	// Check if secondary bitmap is present (bit 1 of primary bitmap)
	hasSecondary := primaryBitmap[0]&0x80 != 0

	// Parse fields based on bitmap
	bitmap := primaryBitmap
	for field := 1; field <= 64; field++ {
		byteIdx := (field - 1) / 8
		bitMask := byte(0x80 >> ((field - 1) % 8))

		if bitmap[byteIdx]&bitMask != 0 {
			// Field is present
			fieldData, newOffset, err := p.parseField(field, data, offset)
			if err != nil {
				return nil, fmt.Errorf("failed to parse field %d: %w", field, err)
			}
			msg.Fields[field] = fieldData
			offset = newOffset
		}
	}

	// Parse secondary bitmap if present
	if hasSecondary {
		if len(data) < offset+8 {
			return nil, fmt.Errorf("message too short for secondary bitmap")
		}
		secondaryBitmap := data[offset : offset+8]
		offset += 8

		for field := 65; field <= 128; field++ {
			byteIdx := (field - 65) / 8
			bitMask := byte(0x80 >> ((field - 65) % 8))

			if secondaryBitmap[byteIdx]&bitMask != 0 {
				// Field is present
				fieldData, newOffset, err := p.parseField(field, data, offset)
				if err != nil {
					return nil, fmt.Errorf("failed to parse field %d: %w", field, err)
				}
				msg.Fields[field] = fieldData
				offset = newOffset
			}
		}
	}

	return msg, nil
}

// parseField parses a single field from the message
func (p *Parser) parseField(field int, data []byte, offset int) ([]byte, int, error) {
	descriptor, ok := p.Fields[field]
	if !ok {
		return nil, offset, fmt.Errorf("unknown field %d", field)
	}

	if descriptor.Variable {
		// Read length indicator
		lengthBytes := 1
		if descriptor.MaxLength > 99 {
			lengthBytes = 2
		}
		if descriptor.MaxLength > 999 {
			lengthBytes = 3
		}

		if len(data) < offset+lengthBytes {
			return nil, offset, fmt.Errorf("not enough data for length indicator of field %d", field)
		}

		var length int
		if lengthBytes == 1 {
			length = int(data[offset])
		} else if lengthBytes == 2 {
			length = int(binary.BigEndian.Uint16(data[offset:]))
		} else {
			length = int(binary.BigEndian.Uint16(data[offset:]))*100 + int(data[offset+2])
		}
		offset += lengthBytes

		if len(data) < offset+length {
			return nil, offset, fmt.Errorf("not enough data for field %d", field)
		}

		fieldData := data[offset : offset+length]
		offset += length

		return fieldData, offset, nil
	}

	// Fixed length field
	if len(data) < offset+descriptor.Length {
		return nil, offset, fmt.Errorf("not enough data for field %d", field)
	}

	fieldData := data[offset : offset+descriptor.Length]
	offset += descriptor.Length

	return fieldData, offset, nil
}

// BuildMessage builds an ISO 8583 message
func (p *Parser) BuildMessage(mti MTI, fields map[int][]byte) ([]byte, error) {
	var buf bytes.Buffer

	// Write MTI
	mtiStr := mti.String()
	buf.WriteString(mtiStr)

	// Calculate bitmap
	bitmap := make([]byte, 16) // 128 bits for primary + secondary
	hasSecondary := false

	for field := range fields {
		if field < 1 || field > 128 {
			return nil, fmt.Errorf("invalid field number: %d", field)
		}
		if field > 64 {
			hasSecondary = true
		}
		byteIdx := (field - 1) / 8
		bitMask := byte(0x80 >> ((field - 1) % 8))
		bitmap[byteIdx] |= bitMask
	}

	// Set secondary bitmap indicator
	if hasSecondary {
		bitmap[0] |= 0x80
	} else {
		// Clear bits for fields 65-128
		for i := 8; i < 16; i++ {
			bitmap[i] = 0
		}
	}

	// Write primary bitmap
	buf.Write(bitmap[:8])

	// Write secondary bitmap if present
	if hasSecondary {
		buf.Write(bitmap[8:])
	}

	// Write fields
	for field := 1; field <= 128; field++ {
		if field > 64 && !hasSecondary {
			break
		}
		if data, ok := fields[field]; ok {
			descriptor, ok := p.Fields[field]
			if !ok {
				return nil, fmt.Errorf("unknown field %d", field)
			}

			if descriptor.Variable {
				// Write length indicator
				lengthBytes := 1
				if descriptor.MaxLength > 99 {
					lengthBytes = 2
				}
				if descriptor.MaxLength > 999 {
					lengthBytes = 3
				}

				if lengthBytes == 1 {
					buf.WriteByte(byte(len(data)))
				} else if lengthBytes == 2 {
					lenBytes := make([]byte, 2)
					binary.BigEndian.PutUint16(lenBytes, uint16(len(data)))
					buf.Write(lenBytes)
				}
			}

			buf.Write(data)
		}
	}

	return buf.Bytes(), nil
}

// HexDump returns a hex dump of the message for debugging
func (m *Message) HexDump() string {
	return hex.Dump(m.Raw)
}

// GetField returns the value of a field as a string
func (m *Message) GetField(field int) (string, bool) {
	data, ok := m.Fields[field]
	if !ok {
		return "", false
	}
	return string(data), true
}

// GetFieldBytes returns the raw bytes of a field
func (m *Message) GetFieldBytes(field int) ([]byte, bool) {
	data, ok := m.Fields[field]
	if !ok {
		return nil, false
	}
	return data, true
}