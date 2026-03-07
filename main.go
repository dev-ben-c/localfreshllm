package main

import (
	"os"

	"github.com/dev-ben-c/localfreshllm/cmd"
	"github.com/dev-ben-c/localfreshllm/render"
)

func main() {
	if err := cmd.Execute(); err != nil {
		render.Errorf("%v", err)
		os.Exit(1)
	}
}
