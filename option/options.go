package option

type Options struct {
	Positional struct {
		Input string
	} `positional-args:"yes"`
	TypeFlag          string   `long:"type" description:"output type" default:"svg" choice:"svg" choice:"dot"`
	Filename          string   `long:"output"`
	NonVariableScalar bool     `long:"non-variable-scalar"`
	VariableScalar    bool     `long:"variable-scalar"`
	Metadata          bool     `long:"metadata"`
	ExecutionStats    bool     `long:"execution-stats"`
	ExecutionSummary  bool     `long:"execution-summary"`
	SerializeResult   bool     `long:"serialize-result"`
	HideScanTarget    bool     `long:"hide-scan-target"`
	ShowQuery         bool     `long:"show-query"`
	ShowQueryStats    bool     `long:"show-query-stats"`
	Full              bool     `long:"full" description:"full output"`
	HideMetadata      []string `long:"hide-metadata"`
}
