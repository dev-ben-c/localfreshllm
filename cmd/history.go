package cmd

import (
	"fmt"

	"github.com/rabidclock/localfreshllm/render"
	"github.com/rabidclock/localfreshllm/session"
)

func runHistory() error {
	store := session.NewStore()
	sessions, err := store.List()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if len(sessions) == 0 {
		render.Infof("No saved conversations.")
		return nil
	}

	fmt.Println(render.AssistantStyle.Render("Saved conversations:"))
	fmt.Println()
	for _, s := range sessions {
		id := render.ModelStyle.Render(s.ID)
		model := render.DimStyle.Render(s.Model)
		ts := render.DimStyle.Render(s.UpdatedAt.Format("2006-01-02 15:04"))
		msgs := render.DimStyle.Render(fmt.Sprintf("%d msgs", len(s.Messages)))
		preview := s.Preview()
		fmt.Printf("  %s  %s  %s  %s\n    %s\n\n", id, model, ts, msgs, preview)
	}

	return nil
}
