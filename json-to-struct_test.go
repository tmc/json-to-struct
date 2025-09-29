package main

// Legacy table-driven test harness has been replaced with txtar-based tests.
// See txtar_test.go for the new test approach.
//
// To run tests:
//   go test                           # Run all txtar tests
//   go test -write-txtar-golden       # Update golden files
//   go test -force-legacy-pattern=X   # Force legacy mode for files matching pattern X