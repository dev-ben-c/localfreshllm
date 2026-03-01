package cmd

import (
	"context"
	"fmt"

	"github.com/rabidclock/localfreshllm/backend"
	"github.com/rabidclock/localfreshllm/render"
)

func runList() error {
	ctx := context.Background()

	fmt.Println(render.AssistantStyle.Render("Ollama models:"))
	ollama := backend.NewOllama()
	models, err := ollama.ListModels(ctx)
	if err != nil {
		render.Infof("  (unavailable: %v)", err)
	} else if len(models) == 0 {
		render.Infof("  (none installed)")
	} else {
		for _, m := range models {
			fmt.Printf("  %s\n", render.ModelStyle.Render(m))
		}
	}

	fmt.Println()
	fmt.Println(render.AssistantStyle.Render("Anthropic models:"))
	anthropic := backend.NewAnthropic()
	models, err = anthropic.ListModels(ctx)
	if err != nil {
		render.Infof("  (unavailable: %v)", err)
	} else {
		for _, m := range models {
			fmt.Printf("  %s\n", render.ModelStyle.Render(m))
		}
	}

	return nil
}
