package templates

import "github.com/valueforvalue/DixieData/internal/uiids"

func uiDebugEnabled() bool {
	return uiids.DebugEnabled()
}

func uiDebugValue() string {
	if uiDebugEnabled() {
		return "true"
	}
	return "false"
}
