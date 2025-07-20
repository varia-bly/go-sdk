module integration-test

go 1.19

require github.com/variably/variably-monorepo/sdks/go/variably v0.0.0

require gopkg.in/yaml.v3 v3.0.1 // indirect

replace github.com/variably/variably-monorepo/sdks/go/variably => ../variably
