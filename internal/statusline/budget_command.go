package statusline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const budgetCommandMarker = "gerado por claude-statusline"

// BudgetCommandPath é o caminho de commands/budget.md dentro do diretório de
// slash commands do Claude Code. Ponto único pra não repetir "budget.md".
func BudgetCommandPath(commandsDir string) string {
	return filepath.Join(commandsDir, "budget.md")
}

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
- Se vier cache_write_error, diga que o consumo está certo mas não deu pra gravar em cache local, e mencione isso rapidamente.
- Se vier error, explique o que falta configurar e aponte a página "Ativando o Claude via LLM Gateway" no Confluence.
`, selfCmd, budgetCommandMarker, selfCmd)
}

// WriteBudgetCommand grava commands/budget.md atomicamente. Preserva
// (written=false) um arquivo que não tenha o marcador: pode ser um comando
// do próprio usuário. Erro de leitura que não seja "arquivo não existe"
// propaga em vez de arriscar sobrescrever algo que não conseguimos nem ler
// (mesma postura de RemoveBudgetCommand).
func WriteBudgetCommand(commandsDir, selfCmd string) (bool, error) {
	path := BudgetCommandPath(commandsDir)
	existing, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		// segue: ainda não existe, tudo bem gravar.
	case err != nil:
		return false, err
	case !strings.Contains(string(existing), budgetCommandMarker):
		return false, nil
	}
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", commandsDir, err)
	}
	if err := writeFileAtomic(path, []byte(BudgetCommandContent(selfCmd)), 0644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// RemoveBudgetCommand apaga commands/budget.md só se foi gerado por nós.
func RemoveBudgetCommand(commandsDir string) (bool, error) {
	path := BudgetCommandPath(commandsDir)
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

// writeFileAtomic escreve data em path via arquivo temporário no mesmo
// diretório seguido de rename, pro leitor nunca ver um budget.md truncado
// (mesmo padrão de writeGatewayCache em gateway_probe.go).
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".claude-statusline-budget-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename file: %w", err)
	}
	return nil
}
