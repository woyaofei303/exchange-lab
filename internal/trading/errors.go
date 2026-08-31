package trading

import "errors"

var (
	// ErrInvalidAsset 表示资产代码或账本精度无效。
	ErrInvalidAsset = errors.New("invalid asset")
	// ErrInvalidScale 表示小数精度超出支持范围。
	ErrInvalidScale = errors.New("invalid scale")
	// ErrInvalidNumber 表示十进制字符串不符合规范格式。
	ErrInvalidNumber = errors.New("invalid decimal number")
	// ErrPrecisionExceeded 表示数值的小数位数超过允许范围。
	ErrPrecisionExceeded = errors.New("precision exceeded")
	// ErrNonPositive 表示要求为正数的值实际为零或负数。
	ErrNonPositive = errors.New("value must be positive")
	// ErrOverflow 表示精确定点值无法用 int64 最小单位表示。
	ErrOverflow = errors.New("fixed-point overflow")
	// ErrInvalidSide 表示订单方向不受支持。
	ErrInvalidSide = errors.New("invalid order side")
	// ErrInvalidOrderType 表示订单类型不受支持。
	ErrInvalidOrderType = errors.New("invalid order type")
	// ErrInvalidOrderStatus 表示订单状态不受支持。
	ErrInvalidOrderStatus = errors.New("invalid order status")
	// ErrInvalidInstrument 表示永续合约规格不一致。
	ErrInvalidInstrument = errors.New("invalid instrument")
	// ErrScaleMismatch 表示数值精度与所属交易品种的精度不同。
	ErrScaleMismatch = errors.New("scale mismatch")
	// ErrStepMismatch 表示价格或数量不是配置步长的整数倍。
	ErrStepMismatch = errors.New("step mismatch")
	// ErrBelowMinimumNotional 表示订单名义价值低于交易品种的最小值。
	ErrBelowMinimumNotional = errors.New("below minimum notional")
)
