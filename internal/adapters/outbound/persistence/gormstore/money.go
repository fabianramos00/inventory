package gormstore

import (
	"fmt"

	"github.com/shopspring/decimal"
)

func moneyToDB(sqlite bool, value decimal.Decimal) any {
	rounded := value.Round(2)
	if sqlite {
		return rounded.Shift(2).IntPart()
	}
	return rounded.StringFixed(2)
}

func moneyFromDB(sqlite bool, value any) (decimal.Decimal, error) {
	switch raw := value.(type) {
	case int64:
		if sqlite {
			return decimal.New(raw, -2), nil
		}
		return decimal.New(raw, 0), nil
	case string:
		return decimal.NewFromString(raw)
	case []byte:
		return decimal.NewFromString(string(raw))
	default:
		return decimal.Zero, fmt.Errorf("unsupported money value %T", value)
	}
}

func moneySQL(sqlite bool, column string) string {
	if sqlite {
		return column
	}
	return column + "::text"
}
