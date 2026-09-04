package statusline

import (
	"fmt"
	"strings"
)

// FormatBRLMicro converte micro-reais (1 real = 1_000_000) em "R$ 1.520,00",
// formato pt-BR, arredondando half-up nos centavos.
func FormatBRLMicro(micro int64) string {
	negative := micro < 0
	if negative {
		micro = -micro
	}
	cents := (micro + 5_000) / 10_000
	reais, rest := cents/100, cents%100
	formatted := fmt.Sprintf("R$ %s,%02d", groupThousands(reais), rest)
	if negative {
		return "-" + formatted
	}
	return formatted
}

func groupThousands(n int64) string {
	digits := fmt.Sprintf("%d", n)
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	head := len(digits) % 3
	if head > 0 {
		b.WriteString(digits[:head])
	}
	for i := head; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// FormatTokens abrevia contagem de tokens: 999, 38.6k, 9.7M (1 casa decimal,
// mesmo formato que o time já vê no script do gateway).
func FormatTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}
