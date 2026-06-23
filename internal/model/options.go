package model

// ConnectOptions controls focus/create behavior for a connect operation.
type ConnectOptions struct {
	NoFocus bool
	Command string
	Root    bool
}
