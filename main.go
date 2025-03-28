package main

import (
	// Adjust import path to your app package
	"The.Blackbook.Archive/app"
)

func main() {
	// Create and run the GUI application
	gui := app.NewGuiApp()
	gui.Run()
}
