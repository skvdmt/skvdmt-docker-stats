package main

import (
	"fmt"
	"os"

	"github.com/skvdmt/skvdmt-docker-stats/internal"
	"github.com/skvdmt/skvdmt-docker-stats/internal/model"
)

func main() {
	var err error
	model.Log, err = model.NewLogger()
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := model.Log.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "%v", err)
		}
	}()
	app, err := internal.NewApp()
	if err != nil {
		model.Log.Error.Error(err.Error())
		os.Exit(1)
	}
	if errs := app.Start(); len(errs) > 0 {
		for _, err := range errs {
			model.Log.Error.Error(err.Error())
		}
		os.Exit(1)
	}
}
