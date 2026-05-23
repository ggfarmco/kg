package main

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type helpCmd struct {
	Name     string     `json:"name"`
	Short    string     `json:"short,omitempty"`
	Use      string     `json:"use,omitempty"`
	Flags    []helpFlag `json:"flags,omitempty"`
	Commands []helpCmd  `json:"commands,omitempty"`
}

type helpFlag struct {
	Name    string `json:"name"`
	Short   string `json:"short,omitempty"`
	Type    string `json:"type"`
	Default string `json:"default,omitempty"`
	Usage   string `json:"usage,omitempty"`
}

func commandTree(c *cobra.Command) helpCmd {
	out := helpCmd{Name: c.Name(), Short: c.Short, Use: c.Use}
	visit := func(f *pflag.Flag) {
		out.Flags = append(out.Flags, helpFlag{
			Name: f.Name, Short: f.Shorthand, Type: f.Value.Type(), Default: f.DefValue, Usage: f.Usage,
		})
	}
	c.LocalFlags().VisitAll(visit)
	if !c.HasParent() {
		c.PersistentFlags().VisitAll(visit)
	}
	for _, sub := range c.Commands() {
		if sub.Hidden {
			continue
		}
		out.Commands = append(out.Commands, commandTree(sub))
	}
	return out
}

func wantsHelpJSON(args []string) bool {
	hasHelp, hasJSON := false, false
	for _, a := range args {
		switch a {
		case "--help", "-h":
			hasHelp = true
		case "--json":
			hasJSON = true
		}
	}
	return hasHelp && hasJSON
}
