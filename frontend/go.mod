// Marker module: carves the frontend tree (notably node_modules, which ships Go
// sources such as flatted/golang) out of the root module so `go build ./...`,
// `go test ./...` and the analyzers never compile npm packages. Nothing builds here.
module github.com/bilal-arikan/tionharness/frontend

go 1.26
