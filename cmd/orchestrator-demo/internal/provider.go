package internal

import (
	"fmt"
	"os"

	_ "github.com/smrobot988-design/Agora/pkg/config"
	"github.com/smrobot988-design/Agora/pkg/llm"
)

// NewMiniMaxProvider creates a MiniMax provider from the MINIMAX_API_KEY environment variable.
// Returns an error if the API key is not set.
func NewMiniMaxProvider() (*llm.MiniMaxProvider, error) {
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("MINIMAX_API_KEY not set (set env or .local.env)")
	}
	return llm.NewMiniMaxProvider(
		llm.MiniMaxWithAPIKey(apiKey),
		llm.MiniMaxWithModel("MiniMax-M2.7"),
		llm.MiniMaxWithMaxTokens(40960),
	), nil
}
