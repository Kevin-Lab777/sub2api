//go:build wireinject
// +build wireinject

// Package gatewayapp exposes the technical account gateway as an embeddable
// application. It deliberately exports only gatewaycore.Runtime; all provider
// accounts and storage wiring remain inside this module.
package gatewayapp

import (
	"context"
	"sync"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/gatewaycore"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/gatewayruntime"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

// Application owns the in-process account gateway and the infrastructure it
// opened. The caller remains responsible for user authentication and billing.
type Application struct {
	Runtime gatewaycore.Runtime
	Cleanup func()
}

// Open loads the Next API technical account gateway from its normal config,
// database, and Redis settings. No HTTP listener is created.
func Open() (*Application, error) {
	return initializeApplication()
}

func initializeApplication() (*Application, error) {
	wire.Build(
		config.ProviderSet,
		technicalProviderSet,
		gatewayruntime.ProviderSet,
		provideCleanup,
		wire.Struct(new(Application), "Runtime", "Cleanup"),
	)
	return nil, nil
}

func providePrivacyClientFactory() service.PrivacyClientFactory {
	return repository.CreatePrivacyReqClient
}

func provideCleanup(
	runtime gatewaycore.Runtime,
	client *ent.Client,
	rdb *redis.Client,
	schedulerSnapshot *service.SchedulerSnapshotService,
	openAIGateway *service.OpenAIGatewayService,
	deferred *service.DeferredService,
	timingWheel *service.TimingWheelService,
) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if runtime != nil {
				_ = runtime.Close(ctx)
			}
			if openAIGateway != nil {
				openAIGateway.CloseOpenAIWSPool()
			}
			if schedulerSnapshot != nil {
				schedulerSnapshot.Stop()
			}
			if deferred != nil {
				deferred.Stop()
			}
			if timingWheel != nil {
				timingWheel.Stop()
			}
			if rdb != nil {
				_ = rdb.Close()
			}
			if client != nil {
				_ = client.Close()
			}
		})
	}
}
