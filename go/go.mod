module github.com/gopherex/schemapb/go

go 1.25.7

require (
	github.com/cbroglie/mustache v1.4.0
	github.com/google/cel-go v0.30.0
	golang.org/x/mod v0.38.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240826202546-f6391c0de4c7
	google.golang.org/protobuf v1.36.11
)

require (
	cel.dev/expr v0.25.1 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/exp v0.0.0-20240823005443-9b4947da3948 // indirect
	golang.org/x/text v0.22.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240826202546-f6391c0de4c7 // indirect
)

// The *Schema convenience methods could panic (an engine-cache GC race);
// v1.8.x-v1.9.2 also let values bypass validation (Duration overflow,
// unrepacked Any on resolve, "__proto__" keys in the TS engine).
retract [v1.7.0, v1.9.2]
