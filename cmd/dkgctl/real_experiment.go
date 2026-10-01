package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/realexperiment"
)

func runRealExperiment(args []string) error {
	flags := flag.NewFlagSet("real-experiment", flag.ContinueOnError)
	scenario := flags.String("scenario", "E0", "fault schedule E0, E1, E2, E4, E5, or E6")
	output := flags.String("output", "", "optional JSON result file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	result, err := realexperiment.Run(*scenario)
	if err != nil {
		return err
	}
	result.Revision = buildRevision()
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if *output != "" {
		if err := os.WriteFile(*output, data, 0o644); err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(os.Stdout, string(data))
	return err
}
