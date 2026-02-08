package output

// Symbols are the status markers a tool prints beside its own lines.
type Symbols struct {
	Success string
	Error   string
	Warning string
	Info    string
}

// DefaultSymbols returns Unicode markers.
func DefaultSymbols() Symbols {
	return Symbols{Success: "✓", Error: "✗", Warning: "⚠", Info: "ℹ"}
}

// ASCIISymbols returns markers for terminals without Unicode.
func ASCIISymbols() Symbols {
	return Symbols{Success: "[ok]", Error: "[err]", Warning: "[!]", Info: "[i]"}
}
