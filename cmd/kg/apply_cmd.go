package main

import "github.com/spf13/cobra"

func newApplyCmd(c *cliCtx) *cobra.Command { return &cobra.Command{Use: "apply", Hidden: true} }
