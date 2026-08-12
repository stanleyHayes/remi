package imports

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type ParseLimits struct {
	MaxBytes     int64
	MaxRows      int
	MaxColumns   int
	MaxCellBytes int
}

func DefaultParseLimits() ParseLimits {
	return ParseLimits{MaxBytes: 20 << 20, MaxRows: 100000, MaxColumns: 200, MaxCellBytes: 10000}
}

type ParsedCSV struct {
	Hash    string
	Headers []string
	Rows    []map[string]string
}

func ParseCSV(reader io.Reader, limits ParseLimits) (ParsedCSV, error) {
	if limits.MaxBytes <= 0 || limits.MaxRows <= 0 || limits.MaxColumns <= 0 || limits.MaxCellBytes <= 0 {
		return ParsedCSV{}, errors.New("CSV parse limits must be positive")
	}
	limited := io.LimitReader(reader, limits.MaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return ParsedCSV{}, fmt.Errorf("read CSV: %w", err)
	}
	if int64(len(data)) > limits.MaxBytes {
		return ParsedCSV{}, errors.New("CSV exceeds maximum file size")
	}
	if !utf8.Valid(data) {
		return ParsedCSV{}, errors.New("CSV must be valid UTF-8")
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	sum := sha256.Sum256(data)
	parser := csv.NewReader(bufio.NewReader(bytes.NewReader(data)))
	parser.FieldsPerRecord = -1
	parser.ReuseRecord = false
	parser.TrimLeadingSpace = true
	headers, err := parser.Read()
	if err == io.EOF {
		return ParsedCSV{}, errors.New("CSV is empty")
	}
	if err != nil {
		return ParsedCSV{}, fmt.Errorf("read CSV header: %w", err)
	}
	if len(headers) == 0 || len(headers) > limits.MaxColumns {
		return ParsedCSV{}, errors.New("CSV has an invalid number of columns")
	}
	seen := map[string]bool{}
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
		if headers[i] == "" || seen[headers[i]] {
			return ParsedCSV{}, errors.New("CSV headers must be non-empty and unique")
		}
		if len(headers[i]) > 128 {
			return ParsedCSV{}, errors.New("CSV header is too long")
		}
		seen[headers[i]] = true
	}
	rows := []map[string]string{}
	for rowNumber := 2; ; rowNumber++ {
		record, readErr := parser.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return ParsedCSV{}, fmt.Errorf("read CSV row %d: %w", rowNumber, readErr)
		}
		if len(record) != len(headers) {
			return ParsedCSV{}, fmt.Errorf("CSV row %d has %d columns; expected %d", rowNumber, len(record), len(headers))
		}
		if len(rows) >= limits.MaxRows {
			return ParsedCSV{}, errors.New("CSV exceeds maximum row count")
		}
		row := make(map[string]string, len(headers))
		empty := true
		for i, value := range record {
			value = strings.TrimSpace(value)
			if len(value) > limits.MaxCellBytes {
				return ParsedCSV{}, fmt.Errorf("CSV row %d column %q is too large", rowNumber, headers[i])
			}
			if value != "" {
				empty = false
			}
			row[headers[i]] = value
		}
		if !empty {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return ParsedCSV{}, errors.New("CSV contains no data rows")
	}
	return ParsedCSV{Hash: hex.EncodeToString(sum[:]), Headers: headers, Rows: rows}, nil
}
