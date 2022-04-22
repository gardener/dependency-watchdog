//go:generate mockgen -package prober -destination=mocks.go github.com/gardener/dependency-watchdog/internal/prober DeploymentScaler,ShootClientCreator
package prober
