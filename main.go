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
	"os"

	"github.com/gardener/dependency-watchdog/cmd"
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
	logger = ctrl.Log.WithName("dwd")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// +kubebuilder:scaffold:scheme
}

func main() {
	var fs flag.FlagSet
	var command cmd.Command

	args := os.Args[1:]
	checkArgs(args)
	parseCommand(args, &fs, &command)

	ctx := ctrl.SetupSignalHandler()

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	err := command.Run(ctx, fs.Args(), logger)
	if err != nil {
		logger.Error(err, "failed to run command %s", command.Name)
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

func parseCommand(args []string, fs *flag.FlagSet, command *cmd.Command) {
	requestedCmdName := args[0]
	command, err := getCommand(requestedCmdName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unexpected error when fetching matching command %s. This should have been checked earlier. Error: %v", requestedCmdName, err)
		os.Exit(2)
	}
	fs = flag.NewFlagSet(requestedCmdName, flag.ContinueOnError)
	fs.Usage = func() {}
	if command.AddFlags != nil {
		command.AddFlags(fs)
	}
	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			cmd.PrintHelp(requestedCmdName, os.Stdout)
			os.Exit(0)
		}
		cmd.PrintHelp(requestedCmdName, os.Stderr)
		os.Exit(2)
	}
}
