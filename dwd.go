// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-logr/logr"

	"github.com/gardener/dependency-watchdog/cmd"
	"go.uber.org/zap/zapcore"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()
	logger logr.Logger
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	args := os.Args[1:]
	checkArgs(args)
	ctx := ctrl.SetupSignalHandler()
	opts := zap.Options{
		Development: false,
		Level:       zapcore.DebugLevel,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	_, command, err := parseCommand(args)
	if err != nil {
		os.Exit(2)
	}
	// initializing global logger
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	// creating root logger from global logger
	logger = ctrl.Log.WithName("dwd")

	mgr, err := command.Run(logger)
	if err != nil {
		logger.Error(err, fmt.Sprintf("failed to run command %s", command.Name))
		os.Exit(1)
	}

	// starting manager
	logger.Info("Starting manager")
	if err = mgr.Start(ctx); err != nil {
		logger.Error(err, "Failed to run the manager")
		os.Exit(1)
	}
}

func checkArgs(args []string) {
	switch {
	case len(args) < 1, args[0] == "-h", args[0] == "--help":
		cmd.PrintCliUsage(os.Stdout)
		os.Exit(0)
	case args[0] == "help":
		if len(args) == 1 {
			_, err := fmt.Fprintf(os.Stderr, "Incorrect usage. To get the CLI usage help use `-h | --help`. To get a command help use `dwd help <command-name>")
			if err != nil {
				return
			}
			os.Exit(2)
		}
		requestedCommand := args[1]
		if _, err := getCommand(requestedCommand); err != nil {
			os.Exit(2)
		}
		cmd.PrintHelp(requestedCommand, os.Stdout)
		os.Exit(0)
	}
}

func getCommand(cmdName string) (*cmd.Command, error) {
	supportedCmdNames := make([]string, len(cmd.Commands))
	for _, cmnd := range cmd.Commands {
		supportedCmdNames = append(supportedCmdNames, cmnd.Name)
		if cmdName == cmnd.Name {
			return cmnd, nil
		}
	}
	return nil, fmt.Errorf("unknown command %s. Supported commands are: %v", cmdName, supportedCmdNames)
}

func parseCommand(args []string) ([]string, *cmd.Command, error) {
	requestedCmdName := args[0]
	command, err := getCommand(requestedCmdName)
	if err != nil {
		_, err := fmt.Fprintf(os.Stderr, "unexpected error when fetching matching command %s. This should have been checked earlier. Error: %v", requestedCmdName, err)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, err
	}
	fs := flag.CommandLine
	fs.Usage = func() {}
	if command.AddFlags != nil {
		command.AddFlags(fs)
	}
	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			cmd.PrintHelp(requestedCmdName, os.Stdout)
			return nil, nil, err
		}
		cmd.PrintHelp(requestedCmdName, os.Stderr)
		return nil, nil, err
	}
	return fs.Args(), command, nil
}
