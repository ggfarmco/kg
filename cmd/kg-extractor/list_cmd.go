package main

import "github.com/spf13/cobra"

type listPlugin struct {
	Name               string   `json:"name"`
	Version            string   `json:"version"`
	Runtime            string   `json:"runtime"`
	Description        string   `json:"description"`
	SupportedLayers    []string `json:"supported_layers,omitempty"`
	SupportedLanguages []string `json:"supported_languages,omitempty"`
}

type listResult struct {
	Plugins []listPlugin `json:"plugins"`
	Errors  []string     `json:"errors,omitempty"`
}

func newListCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List discoverable plugins",
		RunE: func(*cobra.Command, []string) error {
			plugins, errs := discoverPlugins(c.pluginsPath)
			out := listResult{}
			for _, p := range plugins {
				out.Plugins = append(out.Plugins, listPlugin{
					Name: p.Manifest.Name, Version: p.Manifest.Version,
					Runtime: string(p.Manifest.Runtime), Description: p.Manifest.Description,
					SupportedLayers: p.Manifest.SupportedLayers, SupportedLanguages: p.Manifest.SupportedLanguages,
				})
			}
			for _, e := range errs {
				out.Errors = append(out.Errors, e.Error())
			}
			return writeOK(c.stdout, out)
		},
	}
}
