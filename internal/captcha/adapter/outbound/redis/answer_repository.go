package redisadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"github.com/redis/go-redis/v9"
)

// CaptchaAnswerRepository stores slider captcha answers in Redis.
type CaptchaAnswerRepository struct {
	rdb *redis.Client
}

var _ appports.CaptchaAnswerRepository = (*CaptchaAnswerRepository)(nil)

func NewCaptchaAnswerRepository(rdb *redis.Client) *CaptchaAnswerRepository {
	if rdb == nil {
		return nil
	}
	return &CaptchaAnswerRepository{rdb: rdb}
}

func (r *CaptchaAnswerRepository) Save(ctx context.Context, id string, answer domain.SliderAnswer, ttl time.Duration) error {
	if r == nil || r.rdb == nil {
		return fmt.Errorf("captcha answer repository is not configured")
	}

	payload, err := json.Marshal(answer)
	if err != nil {
		return err
	}

	return r.rdb.Set(ctx, answerKey(id), payload, ttl).Err()
}

func (r *CaptchaAnswerRepository) Get(ctx context.Context, id string) (domain.SliderAnswer, error) {
	if r == nil || r.rdb == nil {
		return domain.SliderAnswer{}, fmt.Errorf("captcha answer repository is not configured")
	}

	value, err := r.rdb.Get(ctx, answerKey(id)).Bytes()
	if err == redis.Nil {
		return domain.SliderAnswer{}, domain.ErrCaptchaNotFound
	}
	if err != nil {
		return domain.SliderAnswer{}, err
	}

	var answer domain.SliderAnswer
	if err := json.Unmarshal(value, &answer); err != nil {
		return domain.SliderAnswer{}, err
	}

	return answer, nil
}

func (r *CaptchaAnswerRepository) Delete(ctx context.Context, id string) error {
	if r == nil || r.rdb == nil {
		return fmt.Errorf("captcha answer repository is not configured")
	}

	return r.rdb.Del(ctx, answerKey(id)).Err()
}

func answerKey(id string) string {
	return fmt.Sprintf("captcha:slide:%s", id)
}
