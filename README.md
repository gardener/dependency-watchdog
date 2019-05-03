# dependency-watchdog
When etcd becomes unavailable, the control-plane components go into `CrashloopBackoff`. The control-plane components can remain unavailable for some time even after etcd becomes ready and available. dependency-watchdog helps to alleviate the delay where control plane components remain unavailable by finding the respective pods in CrashloopBackoff and restarting them once etcd becomes ready and available.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Build](#build)
- [Dependency management](#dependency-management)
  - [Updating dependencies](#updating-dependencies)
- [Usage](#usage)

### Prerequisites

Although the following installation instructions are for Mac OS X, similar alternate commands could be found for any Linux distribution

#### Installing [Golang](https://golang.org/) environment

Install the latest version of Golang (at least `v1.9.4` is required). For Mac OS, you could use [Homebrew](https://brew.sh/):

```sh
brew install golang
```

For other OS, please check [Go installation documentation](https://golang.org/doc/install).

Make sure to set your `$GOPATH` environment variable properly (conventionally, it points to `$HOME/go`).

For your convenience, you can add the `bin` directory of the `$GOPATH` to your `$PATH`: `PATH=$PATH:$GOPATH/bin`, but it is not necessarily required.

We use [Dep](https://github.com/golang/dep) for managing golang package dependencies. Please install it
on Mac OS via

```sh
brew install dep
```

On other operating systems, please check the [Dep installation documentation](https://golang.github.io/dep/docs/installation.html) and the [Dep releases page](https://github.com/golang/dep/releases). After downloading the appropriate release in your `$GOPATH/bin` folder, you need to make it executable via `chmod +x <dep-release>` and rename it to dep via `mv dep-<release> dep`.

#### [Golint](https://github.com/golang/lint)

In order to perform linting on the Go source code, please install [Golint](https://github.com/golang/lint):

```bash
go get -u github.com/golang/lint/golint
```

#### Installing `git`

We use `git` as VCS which you would need to install.

On Mac OS run

```sh
brew install git
```

#### Installing `Docker` (Optional)

In case you want to build Docker images, you have to install Docker itself. We recommend using [Docker for Mac OS X](https://docs.docker.com/docker-for-mac/) which can be downloaded from [here](https://download.docker.com/mac/stable/Docker.dmg).

### Build

First, you need to create a target folder structure before cloning and building `dependency-watchdog`.

```sh

mkdir -p ~/go/src/github.com/gardener
cd ~/go/src/github.com/gardener
git clone https://github.com/gardener/dependency-watchdog.git
cd dependency-watchdog
```

To build the binary in your local machine environment, use `make` target `build-local`.

```sh
make build-local
```

This will build the binary `dependency-watchdog` under the `bin` directory.

Next you can make it available to use as shell command by moving the executable to `/usr/local/bin`.

### Dependency management

We use [Dep](https://github.com/golang/dep) to manage golang dependencies.. In order to add a new package dependency to the project, you can perform `dep ensure -add <PACKAGE>` or edit the `Gopkg.toml` file and append the package along with the version you want to use as a new `[[constraint]]`.

#### Updating dependencies

The `Makefile` contains a rule called `revendor` which performs a `dep ensure -update` and a `dep prune` command. This updates all the dependencies to its latest versions (respecting the constraints specified in the `Gopkg.toml` file). The command also installs the packages which do not already exist in the `vendor` folder but are specified in the `Gopkg.toml` (in case you have added new ones).

```sh
make revendor
```

The dependencies are installed into the `vendor` folder which **should be added** to the VCS.

:warning: Make sure you test the code after you have updated the dependencies!

### Usage

Use the `help` option of the `dependency-watchdog` command to show usage details.

```sh
dependency-watchdog --help
Usage of ./bin/dependency-watchdog:
      --alsologtostderr                  log to standard error as well as files
      --config-file string               path to the config file that has the service depenancies (default "config.yaml")
      --kubeconfig string                path to the kube config file (default "kubeconfig.yaml")
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --log_file string                  If non-empty, use this log file
      --log_file_max_size uint           Defines the maximum size a log file can grow to. Unit is megabytes. If the value is 0, the maximum file size is unlimited. (default 1800)
      --logtostderr                      log to standard error instead of files (default true)
      --master string                    The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.
      --skip_headers                     If true, avoid header prefixes in the log messages
      --skip_log_headers                 If true, avoid headers when openning log files
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          number for the log level verbosity
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
      --watch-duration string            The duration to watch dependencies after the service is ready. (default "2m")
```