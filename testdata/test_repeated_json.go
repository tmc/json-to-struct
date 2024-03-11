package test_package

type test_repeated_json struct {
	Bar *float64 `json:"bar,omitempty"`
	Baz *struct {
		Zap *bool `json:"zap,omitempty"`
	} `json:"baz,omitempty"`
	Foo float64 `json:"foo,omitempty"`
}
