package creator

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	"github.com/kokosx/stratum/internal/media"
)

type starterMedia struct {
	iconID   string
	imageIDs []string
	allIDs   []string
}

func createStarterMedia(ctx context.Context, service *media.Service, authorID string, preset PresetID, withImages bool) (starterMedia, error) {
	palette := presetPalette(preset)
	result := starterMedia{}
	upload := func(filename, title, alt string, data []byte) (string, error) {
		asset, err := service.Upload(ctx, filename, authorID, bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		result.allIDs = append(result.allIDs, asset.ID)
		if err := service.UpdateMetadata(ctx, asset.ID, alt, title, "", "Deterministic starter image; replace it in Media at any time."); err != nil {
			return "", err
		}
		return asset.ID, nil
	}
	icon, err := upload("starter-site-icon.png", "Starter site icon", "", geometricPNG(512, 512, palette, 0))
	if err != nil {
		return result, err
	}
	result.iconID = icon
	if err := service.GenerateFaviconVariants(ctx, icon); err != nil {
		return result, err
	}
	if withImages {
		for i := 0; i < 3; i++ {
			id, err := upload("starter-showcase-"+string(rune('1'+i))+".png", "Starter showcase image", "Abstract geometric placeholder image", geometricPNG(1200, 800, palette, i+1))
			if err != nil {
				return result, err
			}
			result.imageIDs = append(result.imageIDs, id)
		}
	}
	return result, nil
}

func cleanupStarterMedia(ctx context.Context, service *media.Service, created starterMedia) {
	for i := len(created.allIDs) - 1; i >= 0; i-- {
		_ = service.Delete(ctx, created.allIDs[i])
	}
}

func geometricPNG(width, height int, palette [3]color.RGBA, variant int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: palette[0]}, image.Point{}, draw.Src)
	margin := width / 6
	offset := variant * width / 30
	draw.Draw(img, image.Rect(margin+offset, height/5, width-margin, height*4/5-offset), &image.Uniform{C: palette[1]}, image.Point{}, draw.Src)
	size := width / 4
	draw.Draw(img, image.Rect(width/2-size/2-offset, height/2-size/2+offset, width/2+size/2-offset, height/2+size/2+offset), &image.Uniform{C: palette[2]}, image.Point{}, draw.Src)
	var buffer bytes.Buffer
	_ = png.Encode(&buffer, img)
	return buffer.Bytes()
}

func presetPalette(id PresetID) [3]color.RGBA {
	switch id {
	case PresetBlog:
		return [3]color.RGBA{{R: 247, G: 243, B: 238, A: 255}, {R: 139, G: 58, B: 58, A: 255}, {R: 38, G: 31, B: 28, A: 255}}
	case PresetPortfolio:
		return [3]color.RGBA{{R: 245, G: 245, B: 244, A: 255}, {R: 17, G: 24, B: 39, A: 255}, {R: 196, G: 181, B: 253, A: 255}}
	case PresetLanding:
		return [3]color.RGBA{{R: 245, G: 243, B: 255, A: 255}, {R: 124, G: 58, B: 237, A: 255}, {R: 253, G: 224, B: 71, A: 255}}
	case PresetProducts:
		return [3]color.RGBA{{R: 241, G: 245, B: 249, A: 255}, {R: 51, G: 65, B: 85, A: 255}, {R: 148, G: 163, B: 184, A: 255}}
	default:
		return [3]color.RGBA{{R: 240, G: 253, B: 250, A: 255}, {R: 15, G: 118, B: 110, A: 255}, {R: 253, G: 186, B: 116, A: 255}}
	}
}
