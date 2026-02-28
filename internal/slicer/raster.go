package slicer

import (
	"image"
	"image/color"
	"math"
	"sort"

	"3dmodels/internal/models"
)

// RasterizeLayer converts contours to a grayscale bitmap for a printer profile.
// offsetX, offsetY are the center coordinates of the model on the build plate (in mm).
func RasterizeLayer(contours []Contour, profile *models.PrinterProfile, offsetX, offsetY float64) *image.Gray {
	width := profile.ResolutionX
	height := profile.ResolutionY

	// Safety check for image dimensions to prevent panic in image.NewGray
	if width <= 0 || height <= 0 || int64(width)*int64(height) > 1000000000 {
		// Return a tiny 1x1 image instead of panicking, or handle error
		// Given the signature, we return an empty image of requested size if valid,
		// but here we must ensure we don't crash.
		if width > 0 && height > 0 && width < 10000 && height < 10000 {
			// small enough but product might be large? logic above covers it.
		} else {
			// Force reasonable limits
			return image.NewGray(image.Rect(0, 0, 1, 1))
		}
	}

	pixelSize := profile.PixelSizeUM / 1000.0 // convert to mm

	img := image.NewGray(image.Rect(0, 0, width, height))

	if len(contours) == 0 {
		return img
	}

	// For each scanline (row), find intersections with contour edges and fill using even-odd rule
	for y := 0; y < height; y++ {
		// Convert pixel Y to mm coordinate
		// Pixel (0,0) is top-left; mm origin is bottom-left of build plate
		mmY := offsetY + (float64(height)/2.0-float64(y))*pixelSize

		// Collect all X intersections for this scanline
		var intersections []float64

		for _, contour := range contours {
			n := len(contour)
			for i := 0; i < n; i++ {
				j := (i + 1) % n
				p1 := contour[i]
				p2 := contour[j]

				// Check if edge crosses this scanline
				if (p1.Y <= mmY && p2.Y > mmY) || (p2.Y <= mmY && p1.Y > mmY) {
					// Calculate X intersection
					t := (mmY - p1.Y) / (p2.Y - p1.Y)
					xIntersect := p1.X + t*(p2.X-p1.X)
					intersections = append(intersections, xIntersect)
				}
			}
		}

		if len(intersections) < 2 {
			continue
		}

		// Sort intersections
		sort.Float64s(intersections)

		// Fill between pairs (even-odd rule)
		for k := 0; k+1 < len(intersections); k += 2 {
			xStart := intersections[k]
			xEnd := intersections[k+1]

			// Convert mm to pixel
			pxStart := int(math.Round((xStart - offsetX + float64(width)/2.0*pixelSize) / pixelSize))
			pxEnd := int(math.Round((xEnd - offsetX + float64(width)/2.0*pixelSize) / pixelSize))

			if pxStart < 0 {
				pxStart = 0
			}
			if pxEnd > width {
				pxEnd = width
			}

			for px := pxStart; px < pxEnd; px++ {
				img.SetGray(px, y, color.Gray{Y: 255})
			}
		}
	}

	return img
}

// RasterizeLayerAA renders at higher resolution and downsamples for anti-aliasing.
// aaLevel must be 2, 4, or 8.
func RasterizeLayerAA(contours []Contour, profile *models.PrinterProfile, offsetX, offsetY float64, aaLevel int) *image.Gray {
	if aaLevel <= 1 {
		return RasterizeLayer(contours, profile, offsetX, offsetY)
	}

	// Create a higher-resolution profile
	hiRes := &models.PrinterProfile{
		BuildWidthMM:  profile.BuildWidthMM,
		BuildDepthMM:  profile.BuildDepthMM,
		BuildHeightMM: profile.BuildHeightMM,
		ResolutionX:   profile.ResolutionX * aaLevel,
		ResolutionY:   profile.ResolutionY * aaLevel,
		PixelSizeUM:   profile.PixelSizeUM / float64(aaLevel),
	}

	hiImg := RasterizeLayer(contours, hiRes, offsetX, offsetY)

	// Downsample
	width := profile.ResolutionX
	height := profile.ResolutionY
	result := image.NewGray(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sum := 0
			for dy := 0; dy < aaLevel; dy++ {
				for dx := 0; dx < aaLevel; dx++ {
					sum += int(hiImg.GrayAt(x*aaLevel+dx, y*aaLevel+dy).Y)
				}
			}
			avg := uint8(sum / (aaLevel * aaLevel))
			result.SetGray(x, y, color.Gray{Y: avg})
		}
	}

	return result
}
