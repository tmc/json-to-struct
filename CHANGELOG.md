# Changelog

## [Unreleased]

### Added

- **Struct Extraction**: New `-extract-structs` flag automatically identifies and extracts repeated nested struct patterns to reduce code duplication
- **Streaming Output Mode**: New `-stream` flag for progressive output display with terminal clearing, ideal for large datasets
- **Roundtrip Validation**: New `-roundtrip` flag generates and executes test programs to verify JSON marshaling/unmarshaling correctness
- **Field Statistics**: New `-stat-comments` flag adds occurrence rates and type distribution information as struct field comments
- **Field Ordering Options**: New `-field-order` flag supports multiple strategies:
  - `alphabetical`: Sort fields alphabetically (default)
  - `encounter`: Maintain order of first encounter
  - `common-first`: Sort by occurrence frequency (most common first)
  - `rare-first`: Sort by occurrence frequency (least common first)
- **Template-Based Generation**: Custom output formatting via `-template` flag with txtar template files
- **NDJSON Support**: Full support for newline-delimited JSON input
- **Performance Profiling**: Added `-cpuprofile` and `-pprof` flags for performance analysis
- **Comprehensive Test Suite**: Added extensive txtar-based test coverage including:
  - Edge case handling (exotic types, unicode, deeply nested structures)
  - Roundtrip validation tests
  - Large payload handling
  - Multi-document NDJSON processing

### Changed

- Replaced `interface{}` with `any` type alias throughout codebase
- Improved field ordering determinism by sorting keys during JSON processing
- Enhanced template rendering with proper function mapping for both default and legacy modes
- Reorganized test files for better clarity (renamed legacy detector files)
- Updated default behavior to use `omitempty` struct tags

### Fixed

- Improved handling of nullable fields with proper pointer type detection
- Better extraction and naming of nested structs with type-based prefixes
- Enhanced legacy mode compatibility with proper template function support

## Previous Releases

See git history for earlier changes.
