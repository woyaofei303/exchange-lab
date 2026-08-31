package trading

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type fixed struct {
	atoms int64
	scale uint8
}

// FormatDecimal renders integer atoms at an explicit decimal scale without
// passing through binary floating point. It is used at persistence and
// transport boundaries that store atoms and scale separately.
func FormatDecimal(atoms int64, scale uint8) (string, error) {
	if scale > maxScale {
		return "", fmt.Errorf("format decimal: %w: scale must not exceed %d", ErrInvalidScale, maxScale)
	}
	return (fixed{atoms: atoms, scale: scale}).String(), nil
}

func parseFixed(value string, scale uint8) (fixed, error) {
	if scale > maxScale {
		return fixed{}, fmt.Errorf("%w: scale must not exceed %d", ErrInvalidScale, maxScale)
	}
	if value == "" || strings.TrimSpace(value) != value {
		return fixed{}, fmt.Errorf("%w: empty or surrounding whitespace", ErrInvalidNumber)
	}

	sign := ""
	unsigned := value
	if unsigned[0] == '-' {
		sign = "-"
		unsigned = unsigned[1:]
	}
	if unsigned == "" {
		return fixed{}, fmt.Errorf("%w: missing digits", ErrInvalidNumber)
	}

	parts := strings.Split(unsigned, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && parts[1] == "") {
		return fixed{}, fmt.Errorf("%w: expected digits with an optional fractional part", ErrInvalidNumber)
	}
	for _, part := range parts {
		for _, character := range part {
			if character < '0' || character > '9' {
				return fixed{}, fmt.Errorf("%w: only base-10 digits are accepted", ErrInvalidNumber)
			}
		}
	}

	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > int(scale) {
		return fixed{}, fmt.Errorf("%w: got %d fractional digits, maximum is %d", ErrPrecisionExceeded, len(fraction), scale)
	}

	digits := strings.TrimLeft(parts[0]+fraction+strings.Repeat("0", int(scale)-len(fraction)), "0")
	if digits == "" {
		digits = "0"
		sign = ""
	}
	atoms, err := strconv.ParseInt(sign+digits, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return fixed{}, fmt.Errorf("%w: %q at scale %d", ErrOverflow, value, scale)
		}
		return fixed{}, fmt.Errorf("%w: %q", ErrInvalidNumber, value)
	}
	return fixed{atoms: atoms, scale: scale}, nil
}

// String renders the exact fixed-scale decimal without exponent notation.
func (v fixed) String() string {
	digits := strconv.FormatInt(v.atoms, 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign = "-"
		digits = strings.TrimPrefix(digits, "-")
	}
	if v.scale == 0 {
		return sign + digits
	}
	width := int(v.scale) + 1
	if len(digits) < width {
		digits = strings.Repeat("0", width-len(digits)) + digits
	}
	point := len(digits) - int(v.scale)
	return sign + digits[:point] + "." + digits[point:]
}
