package trading

import "errors"

var (
	// ErrInvalidAsset indicates an asset code or ledger scale is invalid.
	ErrInvalidAsset = errors.New("invalid asset")
	// ErrInvalidScale indicates a decimal scale is outside the supported range.
	ErrInvalidScale = errors.New("invalid scale")
	// ErrInvalidNumber indicates a decimal string is not in canonical form.
	ErrInvalidNumber = errors.New("invalid decimal number")
	// ErrPrecisionExceeded indicates a value has more fractional digits than allowed.
	ErrPrecisionExceeded = errors.New("precision exceeded")
	// ErrNonPositive indicates a value required to be positive is zero or negative.
	ErrNonPositive = errors.New("value must be positive")
	// ErrOverflow indicates an exact fixed-point value does not fit in int64 atoms.
	ErrOverflow = errors.New("fixed-point overflow")
	// ErrInvalidSide indicates an unsupported order side.
	ErrInvalidSide = errors.New("invalid order side")
	// ErrInvalidOrderType indicates an unsupported order type.
	ErrInvalidOrderType = errors.New("invalid order type")
	// ErrInvalidOrderStatus indicates an unsupported order status.
	ErrInvalidOrderStatus = errors.New("invalid order status")
	// ErrInvalidInstrument indicates a perpetual contract specification is inconsistent.
	ErrInvalidInstrument = errors.New("invalid instrument")
	// ErrScaleMismatch indicates a value was created at a different scale than its instrument.
	ErrScaleMismatch = errors.New("scale mismatch")
	// ErrStepMismatch indicates a price or quantity is not a multiple of its configured step.
	ErrStepMismatch = errors.New("step mismatch")
	// ErrBelowMinimumNotional indicates an order value is below the instrument minimum.
	ErrBelowMinimumNotional = errors.New("below minimum notional")
)
