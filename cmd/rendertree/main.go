package main

import (
	"log"

	"github.com/apstndb/spannerplan/cmd/rendertree/impl"
)

func main() {
	log.Println("deprecation notice: github.com/apstndb/spannerplanviz/cmd/rendertree is deprecated, use github.com/apstndb/spannerplan/cmd/rendertree")
	impl.Main()
}
