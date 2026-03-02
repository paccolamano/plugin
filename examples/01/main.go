package main

import (
	"log"

	"github.com/paccolamano/plugin/plugincmd"

	"github.com/pocketbase/pocketbase"
)

func main() {
	app := pocketbase.New()

	plugincmd.MustRegister(app, app.RootCmd, plugincmd.Config{})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
