package application

import (
	"context"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

type ImageSourceOptions struct {
	PoolSize int
}

type ImageSourceUseCase struct {
	manager appports.RuntimeImageSourceManager
	pool    appports.BackgroundImagePool
	store   appports.RuntimeImageSourceStore
	opts    ImageSourceOptions
}

var _ appports.ImageSourceUseCase = (*ImageSourceUseCase)(nil)

func NewImageSourceUseCase(
	manager appports.RuntimeImageSourceManager,
	pool appports.BackgroundImagePool,
	store appports.RuntimeImageSourceStore,
	opts ImageSourceOptions,
) *ImageSourceUseCase {
	return &ImageSourceUseCase{
		manager: manager,
		pool:    pool,
		store:   store,
		opts:    opts,
	}
}

func (u *ImageSourceUseCase) Status(ctx context.Context) (domain.ImageSourceStatus, error) {
	if u == nil || u.manager == nil || u.pool == nil {
		return domain.ImageSourceStatus{Enabled: false}, nil
	}

	poolSnapshot, err := u.pool.Snapshot(ctx)
	if err != nil {
		poolSnapshot = domain.ImagePoolSnapshot{}
	}

	return u.manager.Status(u.opts.PoolSize, poolSnapshot), nil
}

func (u *ImageSourceUseCase) Validate(ctx context.Context, patch domain.ImageSourcePatch) (domain.ImageSourceValidationResult, error) {
	if u == nil || u.manager == nil || u.pool == nil {
		return domain.ImageSourceValidationResult{}, domain.ErrImagePoolDisabled
	}

	candidate, err := u.manager.BuildCandidateConfig(patch)
	if err != nil {
		return domain.ImageSourceValidationResult{}, err
	}

	if _, err := u.manager.ValidateConfig(ctx, candidate); err != nil {
		return domain.ImageSourceValidationResult{}, err
	}

	return u.manager.ValidationResult(candidate), nil
}

func (u *ImageSourceUseCase) Update(ctx context.Context, patch domain.ImageSourcePatch, triggerRefresh bool) (domain.ImageSourceStatus, error) {
	if u == nil || u.manager == nil || u.pool == nil {
		return domain.ImageSourceStatus{}, domain.ErrImagePoolDisabled
	}

	candidate, err := u.manager.BuildCandidateConfig(patch)
	if err != nil {
		return domain.ImageSourceStatus{}, err
	}

	provider, err := u.manager.ValidateConfig(ctx, candidate)
	if err != nil {
		return domain.ImageSourceStatus{}, err
	}

	if u.store != nil {
		if err := u.store.Save(ctx, candidate); err != nil {
			status, _ := u.Status(ctx)
			return status, &domain.ImageSourcePersistenceError{Err: err}
		}
	}

	u.manager.ApplyConfig(candidate, provider)

	if triggerRefresh {
		err = u.pool.RefreshWithProvider(ctx, provider)
		u.manager.RecordRefreshResult(err)

		status, _ := u.Status(ctx)
		if err != nil {
			return status, &domain.ImageSourceRefreshError{Err: err}
		}
		return status, nil
	}

	return u.Status(ctx)
}

func (u *ImageSourceUseCase) Refresh(ctx context.Context) (domain.ImageSourceStatus, error) {
	if u == nil || u.manager == nil || u.pool == nil {
		return domain.ImageSourceStatus{}, domain.ErrImagePoolDisabled
	}

	err := u.pool.Refresh(ctx)
	u.manager.RecordRefreshResult(err)

	status, _ := u.Status(ctx)
	if err != nil {
		return status, &domain.ImageSourceRefreshError{Err: err}
	}

	return status, nil
}
