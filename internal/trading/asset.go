// Package trading 定义精确的定点数值和合约过滤规则。
package trading

import (
	"fmt"
	"strings"
)

const maxAssetCodeLength = 12
const maxScale = uint8(18)

// Asset 标识一种货币，以及其权威账本表示所使用的小数位数。
type Asset struct {
	code  string
	scale uint8
}

// NewAsset 创建资产，资产代码会统一转换为大写。
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

// Code 返回规范化后的资产代码。
func (a Asset) Code() string {
	return a.code
}

// Scale 返回该资产的账本精度。
func (a Asset) Scale() uint8 {
	return a.scale
}
