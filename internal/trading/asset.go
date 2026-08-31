// Package trading defines exact fixed-point values and contract filters.
package trading

import (
	"fmt"
	"strings"
)

const maxAssetCodeLength = 12
const maxScale = uint8(18)

// Asset identifies a currency and the number of decimal places used by its
// authoritative ledger representation.
type Asset struct {
	code  string
	scale uint8
}

// NewAsset constructs an Asset. Codes are normalized to uppercase.
func NewAsset(code string, scale uint8) (Asset, error) {
	code = strings.ToUpper(code)
	if len(code) < 2 || len(code) > maxAssetCodeLength {
		return Asset{}, fmt.Errorf("%w: code length must be between 2 and %d", ErrInvalidAsset, maxAssetCodeLength)
	}
	for _, character := range code {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return Asset{}, fmt.Errorf("%w: code must contain only ASCII letters and digits", ErrInvalidAsset)
		}
	}
	if scale > maxScale {
		return Asset{}, fmt.Errorf("%w: asset scale must not exceed %d", ErrInvalidScale, maxScale)
	}

	return Asset{code: code, scale: scale}, nil
}

// Code returns the normalized asset code.
func (a Asset) Code() string {
	return a.code
}

// Scale returns the ledger scale for the asset.
func (a Asset) Scale() uint8 {
	return a.scale
}
