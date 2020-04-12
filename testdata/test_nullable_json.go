package test_package

type test_nullable_json struct {
	Foo []struct {
		Bar float64 `json:"bar,omitempty"`
	} `json:"foo,omitempty"`
}
