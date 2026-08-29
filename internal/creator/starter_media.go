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
	socialID string
	imageIDs []string
	allIDs   []string
}

func createStarterMedia(ctx context.Context, service *media.Service, authorID string, paletteID PaletteID, withImages bool) (starterMedia, error) {
	palette := paletteForStyle(paletteID)
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
	social, err := upload("starter-social.png", "Starter social image", "Abstract social preview image", geometricPNG(1200, 630, palette, 7))
	if err != nil {
		return result, err
	}
	result.socialID = social
	if withImages {
		for i := 0; i < 6; i++ {
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
	// Coherent but varied compositions: shift rectangle and accent square per variant.
	// Variant is 1..6; produce deterministic subtle variation so 6 images don't look identical.
	switch variant % 3 {
	case 1:
		margin := width / 6
		offset := variant * width / 36
		draw.Draw(img, image.Rect(margin+offset, height/6, width-margin-offset/2, height*5/6-offset), &image.Uniform{C: palette[1]}, image.Point{}, draw.Src)
		size := width / 5
		draw.Draw(img, image.Rect(width/2-size/2-offset/2, height/2-size/2+offset, width/2+size/2-offset/2, height/2+size/2+offset), &image.Uniform{C: palette[2]}, image.Point{}, draw.Src)
	case 2:
		margin := width / 8
		offset := variant * width / 40
		draw.Draw(img, image.Rect(margin, height/4-offset, width-margin*2, height*3/4+offset/2), &image.Uniform{C: palette[1]}, image.Point{}, draw.Src)
		size := width / 6
		draw.Draw(img, image.Rect(width*3/4-size/2-offset, height/2-size/2, width*3/4+size/2-offset, height/2+size/2), &image.Uniform{C: palette[2]}, image.Point{}, draw.Src)
	default:
		margin := width / 5
		offset := variant * width / 32
		draw.Draw(img, image.Rect(width/2-(width-margin)/2+offset/2, height/5, width/2+(width-margin)/2+offset/2, height*4/5), &image.Uniform{C: palette[1]}, image.Point{}, draw.Src)
		size := width / 4
		draw.Draw(img, image.Rect(width/3-size/2, height/2-size/2-offset, width/3+size/2, height/2+size/2-offset), &image.Uniform{C: palette[2]}, image.Point{}, draw.Src)
	}
	var buffer bytes.Buffer
	_ = png.Encode(&buffer, img)
	return buffer.Bytes()
}

func presetPalette(id PresetID) [3]color.RGBA {
	return paletteForStyle(DefaultPaletteForPreset(id))
}

func paletteForStyle(id PaletteID) [3]color.RGBA {
	switch id {
	case PaletteInk:
		return [3]color.RGBA{{R: 245, G: 245, B: 244, A: 255}, {R: 17, G: 24, B: 39, A: 255}, {R: 196, G: 181, B: 253, A: 255}}
	case PaletteClay:
		return [3]color.RGBA{{R: 247, G: 243, B: 238, A: 255}, {R: 139, G: 58, B: 58, A: 255}, {R: 38, G: 31, B: 28, A: 255}}
	case PaletteForest:
		return [3]color.RGBA{{R: 240, G: 253, B: 250, A: 255}, {R: 15, G: 118, B: 110, A: 255}, {R: 253, G: 186, B: 116, A: 255}}
	case PaletteIndigo:
		return [3]color.RGBA{{R: 245, G: 243, B: 255, A: 255}, {R: 124, G: 58, B: 237, A: 255}, {R: 253, G: 224, B: 71, A: 255}}
	case PaletteOcean:
		return [3]color.RGBA{{R: 240, G: 247, B: 251, A: 255}, {R: 15, G: 76, B: 117, A: 255}, {R: 50, G: 130, B: 184, A: 255}}
	case PaletteSand:
		return [3]color.RGBA{{R: 253, G: 248, B: 243, A: 255}, {R: 160, G: 90, B: 44, A: 255}, {R: 230, G: 210, B: 191, A: 255}}
	case PaletteBerry:
		return [3]color.RGBA{{R: 253, G: 242, B: 245, A: 255}, {R: 139, G: 30, B: 63, A: 255}, {R: 232, G: 184, B: 195, A: 255}}
	case PaletteMidnight:
		return [3]color.RGBA{{R: 14, G: 26, B: 43, A: 255}, {R: 22, G: 35, B: 61, A: 255}, {R: 94, G: 169, B: 255, A: 255}}
	default:
		return [3]color.RGBA{{R: 245, G: 245, B: 244, A: 255}, {R: 17, G: 24, B: 39, A: 255}, {R: 196, G: 181, B: 253, A: 255}}
	}
}
