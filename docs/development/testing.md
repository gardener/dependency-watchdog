
# Testing Strategy and Developer Guideline

Intent of this document is to introduce you (the developer) to the following:
* Category of tests that exists.
* Libraries that are used to write tests.
* Best practices to write tests that are correct, stable, fast and maintainable.
* How to run each category of tests.
* How to debug and test local dependency watchdog changes on local garden cluster.

For any new contributions **tests are a strict requirement**. `Boy Scouts Rule` is followed: If you touch a code for which either no tests exist or coverage is insufficient then it is expected that you will add relevant tests. 




## Setting up Local Garden cluster

A convenient way to test local dependency-watchdog changes is to use a local garden cluster.
To setup a local garden cluster you can follow the [setup-guide](https://github.com/gardener/gardener/blob/master/docs/deployment/getting_started_locally.md).

As part of the local garden installation, a `local` seed will be available. Following resources will be created for dependency watchdog components: