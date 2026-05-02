package captcha

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sync"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/slide"
	"go.uber.org/zap"
)

type SlideGeneratorOptions struct {
	Width        int
	Height       int
	GraphSizeMin int
	GraphSizeMax int
}

type SlideGenerator struct {
	opts   SlideGeneratorOptions
	logger *zap.Logger
	mu     sync.Mutex
}

var _ appports.SliderGenerator = (*SlideGenerator)(nil)

func NewSlideGenerator(opts SlideGeneratorOptions, logger *zap.Logger) *SlideGenerator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SlideGenerator{
		opts:   normalizeSlideGeneratorOptions(opts),
		logger: logger,
	}
}

func (g *SlideGenerator) Generate(ctx context.Context, background []byte) (domain.GeneratedSlider, error) {
	_ = ctx

	if g == nil {
		return domain.GeneratedSlider{}, fmt.Errorf("slide generator is not configured")
	}

	captcha, err := g.buildCaptcha(background)
	if err != nil {
		g.logger.Warn("failed to build captcha with supplied background; using defaults", zap.Error(err))
		captcha, err = g.buildCaptcha(nil)
		if err != nil {
			return domain.GeneratedSlider{}, err
		}
	}

	g.mu.Lock()
	capData, err := captcha.Generate()
	g.mu.Unlock()
	if err != nil {
		return domain.GeneratedSlider{}, err
	}

	block := capData.GetData()
	if block == nil {
		return domain.GeneratedSlider{}, fmt.Errorf("CAPTCHA_DATA_INVALID")
	}

	masterBase64, err := capData.GetMasterImage().ToBase64()
	if err != nil {
		return domain.GeneratedSlider{}, err
	}
	tileBase64, err := capData.GetTileImage().ToBase64()
	if err != nil {
		return domain.GeneratedSlider{}, err
	}

	return domain.GeneratedSlider{
		MasterImage: masterBase64,
		TileImage:   tileBase64,
		Answer: domain.SliderAnswer{
			DX: block.X,
			DY: block.Y,
		},
		TargetY: block.DY,
	}, nil
}

func (g *SlideGenerator) buildCaptcha(background []byte) (slide.Captcha, error) {
	backgrounds := defaultBackgrounds(g.opts.Width, g.opts.Height)
	if len(background) > 0 {
		img, err := png.Decode(bytes.NewReader(background))
		if err != nil {
			return nil, err
		}
		backgrounds = []image.Image{img}
	}

	builder := slide.NewBuilder(
		slide.WithImageSize(option.Size{Width: g.opts.Width, Height: g.opts.Height}),
		slide.WithRangeGraphSize(option.RangeVal{Min: g.opts.GraphSizeMin, Max: g.opts.GraphSizeMax}),
		slide.WithGenGraphNumber(1),
	)
	builder.SetResources(
		slide.WithBackgrounds(backgrounds),
		slide.WithGraphImages(defaultGraphImages(64)),
	)
	return builder.Make(), nil
}

func normalizeSlideGeneratorOptions(opts SlideGeneratorOptions) SlideGeneratorOptions {
	if opts.Width <= 0 {
		opts.Width = 320
	}
	if opts.Height <= 0 {
		opts.Height = 180
	}
	if opts.GraphSizeMin <= 0 {
		opts.GraphSizeMin = 52
	}
	if opts.GraphSizeMax < opts.GraphSizeMin {
		opts.GraphSizeMax = opts.GraphSizeMin + 8
	}
	return opts
}

func defaultBackgrounds(width, height int) []image.Image {
	bg1 := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(bg1, bg1.Bounds(), &image.Uniform{C: color.RGBA{R: 245, G: 248, B: 255, A: 255}}, image.Point{}, draw.Src)

	bg2 := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(bg2, bg2.Bounds(), &image.Uniform{C: color.RGBA{R: 242, G: 250, B: 244, A: 255}}, image.Point{}, draw.Src)

	bg3 := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(bg3, bg3.Bounds(), &image.Uniform{C: color.RGBA{R: 252, G: 245, B: 242, A: 255}}, image.Point{}, draw.Src)

	return []image.Image{bg1, bg2, bg3}
}

func defaultGraphImages(size int) []*slide.GraphImage {
	rect := image.Rect(0, 0, size, size)

	mask := image.NewRGBA(rect)
	drawPuzzleShape(mask, size)

	overlay := image.NewRGBA(rect)
	drawPuzzleBorder(overlay, size, color.RGBA{R: 255, G: 255, B: 255, A: 100})

	shadow := image.NewRGBA(rect)
	drawPuzzleShapeWithStyle(shadow, size, color.RGBA{R: 0, G: 0, B: 0, A: 80})

	return []*slide.GraphImage{{
		OverlayImage: overlay,
		ShadowImage:  shadow,
		MaskImage:    mask,
	}}
}

func drawPuzzleBorder(img *image.RGBA, size int, borderColor color.RGBA) {
	center := size / 2
	mainSize := int(float64(size) * 0.7)
	offset := (size - mainSize) / 2
	bumpSize := mainSize / 4

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
		}
	}

	borderWidth := 2

	for y := offset; y < offset+mainSize; y++ {
		for x := offset; x < offset+mainSize; x++ {
			if x < offset+borderWidth || x >= offset+mainSize-borderWidth ||
				y < offset+borderWidth || y >= offset+mainSize-borderWidth {
				img.Set(x, y, borderColor)
			}
		}
	}

	rightX := offset + mainSize
	rightY := center
	for y := -bumpSize; y <= bumpSize; y++ {
		for x := 0; x <= bumpSize; x++ {
			if x*x+y*y <= bumpSize*bumpSize && x*x+y*y >= (bumpSize-borderWidth)*(bumpSize-borderWidth) {
				px := rightX + x
				py := rightY + y
				if px < size && py >= 0 && py < size {
					img.Set(px, py, borderColor)
				}
			}
		}
	}

	bottomX := center
	bottomY := offset + mainSize
	for y := -bumpSize; y <= 0; y++ {
		for x := -bumpSize; x <= bumpSize; x++ {
			if x*x+y*y <= bumpSize*bumpSize && x*x+y*y >= (bumpSize-borderWidth)*(bumpSize-borderWidth) {
				px := bottomX + x
				py := bottomY + y
				if px >= offset && px < offset+mainSize && py >= offset && py < offset+mainSize {
					img.Set(px, py, borderColor)
				}
			}
		}
	}
}

func drawPuzzleShape(img *image.RGBA, size int) {
	center := size / 2
	mainSize := int(float64(size) * 0.7)
	offset := (size - mainSize) / 2
	bumpSize := mainSize / 4

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
		}
	}

	for y := offset; y < offset+mainSize; y++ {
		for x := offset; x < offset+mainSize; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	rightX := offset + mainSize
	rightY := center
	for y := -bumpSize; y <= bumpSize; y++ {
		for x := 0; x <= bumpSize; x++ {
			if x*x+y*y <= bumpSize*bumpSize {
				px := rightX + x
				py := rightY + y
				if px < size && py >= 0 && py < size {
					img.Set(px, py, color.RGBA{R: 255, G: 255, B: 255, A: 255})
				}
			}
		}
	}

	bottomX := center
	bottomY := offset + mainSize
	for y := -bumpSize; y <= 0; y++ {
		for x := -bumpSize; x <= bumpSize; x++ {
			if x*x+y*y <= bumpSize*bumpSize {
				px := bottomX + x
				py := bottomY + y
				if px >= offset && px < offset+mainSize && py >= offset && py < offset+mainSize {
					img.Set(px, py, color.RGBA{R: 0, G: 0, B: 0, A: 0})
				}
			}
		}
	}
}

func drawPuzzleShapeWithStyle(img *image.RGBA, size int, fillColor color.RGBA) {
	center := size / 2
	mainSize := int(float64(size) * 0.7)
	offset := (size - mainSize) / 2
	bumpSize := mainSize / 4

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
		}
	}

	for y := offset; y < offset+mainSize; y++ {
		for x := offset; x < offset+mainSize; x++ {
			img.Set(x, y, fillColor)
		}
	}

	rightX := offset + mainSize
	rightY := center
	for y := -bumpSize; y <= bumpSize; y++ {
		for x := 0; x <= bumpSize; x++ {
			if x*x+y*y <= bumpSize*bumpSize {
				px := rightX + x
				py := rightY + y
				if px < size && py >= 0 && py < size {
					img.Set(px, py, fillColor)
				}
			}
		}
	}

	bottomX := center
	bottomY := offset + mainSize
	for y := -bumpSize; y <= 0; y++ {
		for x := -bumpSize; x <= bumpSize; x++ {
			if x*x+y*y <= bumpSize*bumpSize {
				px := bottomX + x
				py := bottomY + y
				if px >= offset && px < offset+mainSize && py >= offset && py < offset+mainSize {
					img.Set(px, py, color.RGBA{R: 0, G: 0, B: 0, A: 0})
				}
			}
		}
	}
}
