package statusline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const budgetCommandMarker = "gerado por claude-statusline"

// BudgetCommandContent é o slash command /budget do Claude Code: roda o
// binário em modo --json e pede pro modelo explicar em português.
func BudgetCommandContent(selfCmd string) string {
	if strings.Contains(selfCmd, " ") {
		selfCmd = `"` + selfCmd + `"`
	}
	return fmt.Sprintf(`---
description: Mostra seu limite e consumo no gateway LLM da Superlógica
allowed-tools: Bash(%s budget:*)
---
<!-- %s; reinstalar sobrescreve, apagar desinstala -->
Budget do usuário no gateway:

!`+"`"+`%s budget --json`+"`"+`

Apresente em português, sem tabela:
- Quanto já gastou e qual o teto, em reais (spent_brl e limit_brl já vêm em reais; o campo raw traz os valores originais em micro-reais). Diga o percentual (pct).
- Quando a janela reseta (window_end) e qual o período (period).
- Se scope for license, explique que não há teto individual e que os valores são o agregado da licença, não só dele.
- Se base_limit_brl for maior que limit_brl, explique que o teto individual está limitado pelo teto da licença.
- Se exceeded for true, diga que o acesso está bloqueado até a data de reset.
- Se vier stale true, avise que os dados são de fetched_at porque o gateway não respondeu agora.
- Se vier error, explique o que falta configurar e aponte a página "Ativando o Claude via LLM Gateway" no Confluence.
`, selfCmd, budgetCommandMarker, selfCmd)
}

// WriteBudgetCommand grava commands/budget.md. Preserva (written=false) um
// arquivo que não tenha o marcador: pode ser um comando do próprio usuário.
func WriteBudgetCommand(commandsDir, selfCmd string) (bool, error) {
	path := filepath.Join(commandsDir, "budget.md")
	if existing, err := os.ReadFile(path); err == nil && !strings.Contains(string(existing), budgetCommandMarker) {
		return false, nil
	}
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", commandsDir, err)
	}
	if err := os.WriteFile(path, []byte(BudgetCommandContent(selfCmd)), 0644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// RemoveBudgetCommand apaga commands/budget.md só se foi gerado por nós.
func RemoveBudgetCommand(commandsDir string) (bool, error) {
	path := filepath.Join(commandsDir, "budget.md")
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !strings.Contains(string(existing), budgetCommandMarker) {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}
