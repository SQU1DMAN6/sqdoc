package main

import (
	"fmt"
	"os"

	"sqdoc/internal/app"
)

func main() {
	application := app.New()
	if err := application.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "SIDE failed: %v\n", err)
		os.Exit(1)
	}
}
