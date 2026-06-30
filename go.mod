module github.com/apstndb/spannerplanviz

go 1.23.0

retract v0.7.1 // Incorrect patch release; breaking changes belong in v0.8.x.

toolchain go1.24.0

require (
	cloud.google.com/go/spanner v1.48.0
	github.com/MakeNowJust/heredoc/v2 v2.0.1
	github.com/apstndb/spannerplan v0.1.11
	github.com/goccy/go-graphviz v0.2.10
	github.com/google/go-cmp v0.5.9
	github.com/jessevdk/go-flags v1.6.1
	google.golang.org/protobuf v1.33.0
	sigs.k8s.io/yaml v1.2.0
)

require (
	github.com/apstndb/go-tabwrap v0.1.3 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/disintegration/imaging v1.6.2 // indirect
	github.com/flopp/go-findfont v0.1.0 // indirect
	github.com/fogleman/gg v1.3.0 // indirect
	github.com/goccy/go-yaml v1.17.1 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/samber/lo v1.53.0 // indirect
	github.com/tetratelabs/wazero v1.10.1 // indirect
	golang.org/x/image v0.21.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto v0.0.0-20230530153820-e85fd2cbaebc // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230530153820-e85fd2cbaebc // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230530153820-e85fd2cbaebc // indirect
	google.golang.org/grpc v1.56.3 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
