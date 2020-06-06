package test_package

type test_repeated_json struct {
	Foo float64 `json:"foo,omitempty"`
	Bar float64 `json:"bar,omitempty"`
	Baz struct {
		Zap bool `json:"zap,omitempty"`
	} `json:"baz,omitempty"`
}
