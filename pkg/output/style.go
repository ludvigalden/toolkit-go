package output

import "github.com/ludvigalden/toolkit-go/pkg/output/internal/textfmt"

// The ANSI palette diagnostic lines use, for tools that tint their own
// text to match. Apply them only when Args.Color is set.
const (
	StyleReset   = textfmt.Reset
	StyleDim     = textfmt.Dim
	StyleInfo    = textfmt.Info
	StyleWarn    = textfmt.Warn
	StyleError   = textfmt.Error
	StyleSuccess = textfmt.Success
	StyleKey     = textfmt.Key
)
