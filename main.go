package main

import (
	"os"

	"github.com/rabidclock/localfreshllm/cmd"
	"github.com/rabidclock/localfreshllm/render"
)

func main() {
	if err := cmd.Execute(); err != nil {
		render.Errorf("%v", err)
		os.Exit(1)
	}
}
