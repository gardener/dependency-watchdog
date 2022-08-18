/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"github.com/go-logr/logr"
	"os"

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
		Development: true,
		Level:       zapcore.DebugLevel,
	}
	opts.BindFlags(flag.CommandLine)
	_, command, err := parseCommand(args)
	if err != nil {
		os.Exit(2)
	}
	// flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	logger = ctrl.Log.WithName("dwd")

	mgr, err := command.Run(ctx, logger)
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
			fmt.Fprintf(os.Stderr, "Incorrect usage. To get the CLI usage help use `-h | --help`. To get a command help use `dwd help <command-name>")
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
	for _, cmd := range cmd.Commands {
		supportedCmdNames = append(supportedCmdNames, cmd.Name)
		if cmdName == cmd.Name {
			return cmd, nil
		}
	}
	return nil, fmt.Errorf("unknown command %s. Supported commands are: %v", cmdName, supportedCmdNames)
}

func parseCommand(args []string) ([]string, *cmd.Command, error) {
	requestedCmdName := args[0]
	command, err := getCommand(requestedCmdName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unexpected error when fetching matching command %s. This should have been checked earlier. Error: %v", requestedCmdName, err)
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
