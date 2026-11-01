// Package adapter turns each assistant's input into a model.Session.
package adapter

import "github.com/milczarekIT/rainbow-statusline/internal/model"

// Parse never fails: anything it cannot read stays unknown in the session.
func Parse(agent string, stdin []byte) model.Session {
	switch agent {
	case "claude":
		return parseClaude(stdin)
	case "copilot":
		return parseCopilot(stdin)
	}
	return model.Session{}
}
