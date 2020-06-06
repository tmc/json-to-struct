package test_package

type test_nested_json struct {
	Baz []float64 `json:"baz,omitempty"`
	Foo struct {
		Bar float64 `json:"bar,omitempty"`
	} `json:"foo,omitempty"`
}
